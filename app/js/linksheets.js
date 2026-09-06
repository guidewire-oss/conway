import { authFetch } from './auth.js';
import { openModal, closeModal } from './modal.js';
import { esc } from './order.js';
import { icon } from './icons.js';

const date = value => value ? new Date(value * 1000).toLocaleString() : 'Not checked yet';
const list = values => (values || []).map(value => `<li>${esc(value)}</li>`).join('');
function inputTable(kind, candidate) {
  if (kind === 'teams') return `<div style="overflow:auto"><table><thead><tr><th>Team</th><th>Developers</th><th>Pairing</th><th>Tracks</th><th>Site</th><th>Capacity loss</th></tr></thead><tbody>${(candidate.teams || []).map(team => `<tr><td>${esc(team.name)}</td><td>${esc(team.devs)}</td><td>${team.pairs ? 'Yes' : 'No'}</td><td>${esc(team.tracks || (team.pairs ? Math.ceil(team.devs / 2) : team.devs))}</td><td>${esc(team.site || 'Not set')}</td><td>${team.capacityLoss ? `${esc(team.capacityLoss * 100)}%` : 'Plan default'}</td></tr>`).join('')}</tbody></table></div>`;
  return (candidate.initiatives || []).map(it => `<section style="border-top:1px solid var(--border,#ccc);padding:.75rem 0"><h3>${esc(it.name)}</h3><p>Priority: ${esc(it.statedPriority || 'Unranked')}${it.priorityLocked ? ' (locked)' : ''} · target: ${esc(it.targetDate || 'Not set')}${it.dateLocked ? ' (locked)' : ''} · earliest start: ${esc(it.earliestStart || 'Not set')}<br>Predecessor initiatives: ${esc((it.afterInitiatives || []).join(', ') || 'None')}<br>Epic bindings: ${esc((it.epicKeys || []).join(', ') || 'None')}</p><div style="overflow:auto"><table><thead><tr><th>Team</th><th>Estimate (weeks)</th><th>Team dependencies</th></tr></thead><tbody>${Object.entries(it.work || {}).map(([team, work]) => `<tr><td>${esc(team)}</td><td>${!work.inPath ? 'Not assigned' : work.estimated ? esc(work.weeks) : 'Unknown — estimate needed'}</td><td>${esc((work.dependsOn || []).join(', ') || 'None')}</td></tr>`).join('')}</tbody></table></div></section>`).join('');
}

// specs/023-linked-google-sheets.md:155: captures are observations, not Google's
// complete revision history. All apply actions use a freshly loaded preview.
export async function openLinkedSheets(planID, onApplied = async () => {}) {
  let overlay = document.getElementById('linked-sheets-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'linked-sheets-overlay';
    overlay.className = 'modal-overlay';
    overlay.hidden = true;
    document.body.appendChild(overlay);
  }
  const token = Symbol('linked-sheets');
  overlay.sheetToken = token;
  overlay.setAttribute('aria-labelledby', 'linked-sheets-title');
  const base = `/api/plan/${encodeURIComponent(planID)}/sources`;
  let ticket = 0;
  let closed = false;
  overlay.sheetCloseCleanup?.();
  const closing = () => { closed = true; ticket++; };
  overlay.addEventListener('hide.bs.modal', closing);
  overlay.sheetCloseCleanup = () => overlay.removeEventListener('hide.bs.modal', closing);
  const live = () => overlay.sheetToken === token && !closed && !overlay.hidden;
  const frame = (title, body) => {
    let box = overlay.querySelector('.modal-box');
    if (!box) { box = document.createElement('div'); box.className = 'modal-box'; overlay.appendChild(box); }
    box.style.cssText = 'width:min(920px,95vw);max-width:95vw;max-height:90vh;overflow:auto;overflow-wrap:anywhere';
    box.innerHTML = `<div class="modal-head"><h2 id="linked-sheets-title" tabindex="-1">${esc(title)}</h2><button type="button" data-close>${icon('close')}Close</button></div><div data-sheet-body>${body}</div><p data-sheet-status role="status" aria-live="polite"></p>`;
    overlay.querySelector('[data-close]').addEventListener('click', () => { closing(); closeModal(overlay); });
    if (live()) overlay.querySelector('h2').focus();
  };
  const status = (message, error = false) => {
    if (!live()) return;
    const el = overlay.querySelector('[data-sheet-status]');
    el.textContent = message;
    el.setAttribute('role', error ? 'alert' : 'status');
  };
  async function request(path, options = {}) {
    const response = await authFetch(path, options);
    if (!response.ok) throw new Error((await response.text()).slice(0, 600) || `Request failed (${response.status}).`);
    return response.json();
  }
  async function action(button, work) {
    button.disabled = true;
    try { await work(); } catch (error) { status(error.message || 'Could not reach the server. Try again.', true); }
    finally { if (button.isConnected) button.disabled = false; }
  }
  function sourcePath(source) { return `${base}/${encodeURIComponent(source.id)}`; }
  async function load() {
    const ownTicket = ++ticket;
    const result = await request(base);
    if (!live() || ownTicket !== ticket) return;
    const sources = result.sources || [];
    frame('Linked Google Sheets', `<p>Link one sheet range for this plan’s teams and one for its initiatives. Changes are captured for review; saved agreements stay unchanged.</p>
      <p class="hint">History contains Conway captures made during checks. Changes between checks are not recorded, and this is not Google’s complete revision history.</p>
      ${result.configured ? `<p>Share each spreadsheet as a viewer with <strong>${esc(result.serviceAccountEmail || 'the configured service account')}</strong>.</p>` : '<p role="alert">Google Sheets is not configured. Ask an administrator to set up the server service account. Existing captures remain available.</p>'}
      <div data-sources>${sources.length ? sources.map(source => `<section class="linked-source" style="border-top:1px solid var(--border,#ccc);padding:1rem 0" data-source="${esc(source.id)}">
        <h3>${source.kind === 'teams' ? 'Team roster' : 'Initiatives'} · ${esc(source.status)}</h3>
        <p><a href="${esc(source.spreadsheetUrl)}" target="_blank" rel="noopener noreferrer">Open spreadsheet</a> · range <strong>${esc(source.range)}</strong></p>
        <p>Mode: ${source.mode === 'auto_apply' ? 'Apply safe updates automatically' : 'Review every update'} · checks every ${esc(source.pollMinutes)} minutes<br>Last check: ${esc(date(source.lastCheckedAt))}</p>
        ${source.lastError ? `<p role="alert">${esc(source.lastError)}</p>` : ''}
        ${source.latestVersionId && source.latestVersionId !== source.appliedVersionId ? '<p><strong>A captured version is awaiting review.</strong></p>' : ''}
        <div style="display:flex;flex-wrap:wrap;gap:.5rem">
          <button type="button" data-history>Captured versions</button>
          ${source.status !== 'disconnected' ? `<button type="button" data-check ${!result.configured ? 'disabled' : ''}>Check now</button><button type="button" data-pause>${source.status === 'paused' ? 'Resume checks' : 'Pause checks'}</button><button type="button" data-disconnect>Disconnect</button>` : ''}
        </div>
        ${source.status !== 'disconnected' ? `<form data-settings style="display:flex;gap:.5rem;align-items:end;flex-wrap:wrap;margin-top:1rem"><label>Update mode<select name="mode"><option value="review" ${source.mode === 'review' ? 'selected' : ''}>Review every update</option><option value="auto_apply" ${source.mode === 'auto_apply' ? 'selected' : ''}>Apply safe updates automatically</option></select></label><label>Check interval (minutes)<input name="pollMinutes" type="number" min="5" max="1440" required value="${esc(source.pollMinutes)}"></label><button type="submit">Save source settings</button></form>` : ''}
      </section>`).join('') : '<p>No sheets are linked to this plan.</p>'}</div>
      ${result.configured ? `<details ${sources.every(s => s.status === 'disconnected') ? 'open' : ''}><summary>Link a sheet range</summary><form data-link style="display:grid;gap:.75rem;margin-top:1rem">
      <label>Planning input<select name="kind"><option value="teams">Team roster</option><option value="initiatives">Initiatives</option></select></label>
      <label>Google Sheets link<input name="spreadsheetUrl" type="url" required placeholder="https://docs.google.com/spreadsheets/d/…/edit"></label>
      <label>Tab name or A1 range<input name="range" required maxlength="300" placeholder="Teams or 'Planning inputs'!A1:Z500"></label>
      <p class="hint">Include the header row. Use the same roster and initiative columns as Conway’s upload templates. Link the team roster first if this plan has no teams.</p>
      <label>Update mode<select name="mode"><option value="review">Review every update (default)</option><option value="auto_apply">Apply safe updates automatically</option></select></label>
      <p class="hint">Automatic mode applies only valid updates without scope removal, while the working plan still matches this source’s checkpoint. Local edits require review.</p>
      <label>Check interval (minutes)<input name="pollMinutes" type="number" min="5" max="1440" value="15" required></label><button type="submit" class="primary">Link and capture</button></form></details>` : ''}`);
    for (const source of sources) {
      const section = [...overlay.querySelectorAll('[data-source]')].find(el => el.dataset.source === source.id);
      section.querySelector('[data-history]').addEventListener('click', ev => action(ev.currentTarget, () => history(source)));
      section.querySelector('[data-check]')?.addEventListener('click', ev => action(ev.currentTarget, async () => {
        status('Checking the linked range…');
        const checked = await request(`${sourcePath(source)}/check`, { method: 'POST', body: '{}' });
        if (!live()) return;
        if (checked.applied) await onApplied();
        await load();
        status(checked.source.lastError || (checked.applied ? 'Captured changes applied to the working plan.' : checked.unchanged ? 'The sheet content is unchanged.' : 'A new capture is ready to inspect.'), !!checked.source.lastError);
      }));
      section.querySelector('[data-pause]')?.addEventListener('click', ev => action(ev.currentTarget, async () => {
        await request(sourcePath(source), { method: 'PATCH', body: JSON.stringify({ status: source.status === 'paused' ? 'active' : 'paused' }) });
        await load();
      }));
      section.querySelector('[data-disconnect]')?.addEventListener('click', ev => action(ev.currentTarget, async () => {
        await request(sourcePath(source), { method: 'DELETE' });
        await load(); status('Source disconnected. Its captured history remains available.');
      }));
      section.querySelector('[data-settings]')?.addEventListener('submit', ev => {
        ev.preventDefault(); const form = ev.currentTarget;
        action(form.querySelector('button'), async () => {
          await request(sourcePath(source), { method: 'PATCH', body: JSON.stringify({ mode: form.elements.mode.value, pollMinutes: Number(form.elements.pollMinutes.value) }) });
          await load(); status('Source settings saved.');
        });
      });
    }
    overlay.querySelector('[data-link]')?.addEventListener('submit', ev => {
      ev.preventDefault(); const form = ev.currentTarget;
      action(form.querySelector('button'), async () => {
        status('Connecting and capturing the sheet…');
        const linked = await request(base, { method: 'POST', body: JSON.stringify({ kind: form.elements.kind.value, spreadsheetUrl: form.elements.spreadsheetUrl.value, range: form.elements.range.value, mode: form.elements.mode.value, pollMinutes: Number(form.elements.pollMinutes.value) }) });
        if (!live()) return;
        if (linked.applied) await onApplied();
        await load(); status(linked.source.lastError || 'Source linked. Inspect its captured version before applying changes.', !!linked.source.lastError);
      });
    });
  }
  async function history(source) {
    const ownTicket = ++ticket;
    const result = await request(`${sourcePath(source)}/versions`);
    if (!live() || ownTicket !== ticket) return;
    frame(`${source.kind === 'teams' ? 'Team roster' : 'Initiatives'} captured versions`, `<button type="button" data-back>Back to linked sources</button><p>Each entry is a Conway observation. Applying an earlier capture records a restore; it does not rewrite history or your agreement.</p><div style="overflow:auto"><table><thead><tr><th>Captured</th><th>Rows</th><th>Validation</th><th>Application</th><th>Review</th></tr></thead><tbody>${(result.versions || []).map(v => `<tr><td>${esc(date(v.capturedAt))}<br><small>${esc(v.id)}</small></td><td>${esc(v.count)}</td><td>${v.valid ? 'Valid when captured' : 'Invalid when captured'}${v.removals?.length ? ' · removes scope' : ''}</td><td>${v.id === source.appliedVersionId ? 'Last applied' : v.id === source.latestVersionId ? 'Latest capture' : 'Earlier capture'}</td><td><button type="button" data-version="${esc(v.id)}">Inspect</button></td></tr>`).join('')}</tbody></table></div>${result.versions?.length ? '' : '<p>No content has been captured yet. Check the source permissions and range.</p>'}`);
    overlay.querySelector('[data-back]').addEventListener('click', ev => action(ev.currentTarget, load));
    overlay.querySelectorAll('[data-version]').forEach(button => button.addEventListener('click', () => action(button, () => preview(source, button.dataset.version))));
  }
  async function preview(source, versionID) {
    const ownTicket = ++ticket;
    const result = await request(`${sourcePath(source)}/versions/${encodeURIComponent(versionID)}`);
    if (!live() || ownTicket !== ticket) return;
    const v = result.version, candidate = result.preview || v.parsed || {};
    const removals = candidate.removals || [], errors = candidate.errors || [];
    const historicalErrors = v.errors || [];
    const historicalWarnings = v.warnings || [];
    const valid = result.applyable ?? (candidate.count > 0 && !errors.length);
    frame('Review captured changes', `<button type="button" data-back>Back to captured versions</button><p>${esc(source.range)} · captured ${esc(date(v.capturedAt))}<br>Capture ${esc(v.id)}</p><p>This ${source.kind === 'teams' ? 'roster' : 'initiative matrix'} contains <strong>${esc(candidate.count ?? v.count)}</strong> records. Applying replaces this plan’s ${source.kind === 'teams' ? 'team roster' : 'initiatives'} with the validated capture. Its other inputs and saved agreements are retained.</p>
      ${historicalErrors.length || historicalWarnings.length || !v.valid ? `<details><summary>Validation recorded at capture time</summary><p>${v.valid ? 'Valid' : 'Invalid'} when captured. These original diagnostics remain part of the capture history. The current validation below determines whether you can apply it now.</p>${historicalErrors.length ? `<h3>Original errors</h3><ul>${list(historicalErrors)}</ul>` : ''}${historicalWarnings.length ? `<h3>Original warnings</h3><ul>${list(historicalWarnings)}</ul>` : ''}</details>` : ''}
      ${errors.length ? `<div role="alert"><h3>Cannot apply to the current plan</h3><ul>${list([...new Set(errors)])}</ul></div>` : valid ? '<p>Current validation passed. This capture can be applied through this explicit review.</p>' : ''}
      ${(candidate.warnings || []).length ? `<h3>Review notes</h3><ul>${list(candidate.warnings)}</ul>` : ''}
      ${removals.length ? `<h3>Scope that will be removed</h3><ul>${list(removals)}</ul><label><input type="checkbox" data-removals> I reviewed and accept these removals.</label>` : ''}
      <details open><summary>Inspect proposed planning inputs</summary>${inputTable(source.kind, candidate)}</details>
      <details><summary>Inspect original captured cells</summary><p>Showing up to 100 rows and 24 columns. Download the complete captured cells to inspect a larger range.</p><button type="button" data-download>Download captured cells (JSON)</button><div style="overflow:auto;max-height:28rem"><table><tbody>${(v.rows || []).slice(0, 100).map(row => `<tr>${row.slice(0, 24).map(value => `<td>${esc(value)}</td>`).join('')}</tr>`).join('')}</tbody></table></div></details>
      <p class="hint">This preview is checked against the current working plan. Any later local edit will refuse this application and require a fresh preview.</p><button type="button" class="primary" data-apply ${!valid || removals.length ? 'disabled' : ''}>${v.id === source.latestVersionId ? 'Apply capture to working plan' : 'Restore this capture to working plan'}</button>`);
    overlay.querySelector('[data-back]').addEventListener('click', ev => action(ev.currentTarget, () => history(source)));
    overlay.querySelector('[data-download]').addEventListener('click', () => {
      const url = URL.createObjectURL(new Blob([JSON.stringify(v.rows || [], null, 2)], { type: 'application/json' }));
      const link = document.createElement('a'); link.href = url; link.download = 'conway-sheet-capture.json'; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    });
    const apply = overlay.querySelector('[data-apply]');
    overlay.querySelector('[data-removals]')?.addEventListener('change', ev => { apply.disabled = !valid || !ev.currentTarget.checked; });
    apply.addEventListener('click', () => action(apply, async () => {
      status('Applying the reviewed capture…');
      await request(`${sourcePath(source)}/apply`, { method: 'POST', body: JSON.stringify({ versionId: v.id, expectedFingerprint: result.planFingerprint, allowRemovals: !!overlay.querySelector('[data-removals]')?.checked }) });
      if (!live()) return;
      await onApplied(); await load(); status('Captured inputs applied. Saved agreements and earlier captures are unchanged.');
    }));
  }
  frame('Linked Google Sheets', '<p>Loading linked sources…</p>');
  openModal(overlay);
  try { await load(); return live(); } catch (error) { status(error.message || 'Could not load linked sources. Close and try again.', true); return false; }
}
