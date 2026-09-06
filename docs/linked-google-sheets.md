# Configure linked Google Sheets

Managers use a linked source inside a saved plan to maintain its teams or
initiatives from a spreadsheet. The [user guide](../app/docs.html#linked-sheets)
explains review, automatic application, conflicts and captured versions.

## Enable the connection

1. In a Google Cloud project, enable the Google Sheets API and create a service
   account dedicated to Conway. Domain-wide delegation is unnecessary.
2. Obtain its JSON credentials and mount the file into the Conway server as a
   deployment secret, readable only by the server process. Keep it outside the
   repository, workbook uploads and browser assets.
3. Set `CONWAY_GOOGLE_CREDENTIALS_FILE` to that mounted file's path and restart
   the server. Every instance running periodic checks needs the configuration.
4. Share each intended spreadsheet with the service-account email as a viewer.
   Managers can copy that email from the Linked Google Sheets dialog.

The server requests the
[`spreadsheets.readonly` scope](https://developers.google.com/workspace/sheets/api/scopes).
That scope covers spreadsheets the identity can access; it is not a per-tab
security boundary. Use sharing permissions to limit access. Credentials stay on
the server, and Conway reads through fixed Google API endpoints. This connection
does not grant access to private spreadsheets merely from knowing their links.

See Google's [service-account authentication guide](https://developers.google.com/identity/protocols/oauth2/service-account)
for credential creation and management. Rotate credentials through your secret
deployment process, update the mounted file and restart Conway.

## Data and authorization

Linking and applying require a manager with access to the plan (its owner or an
administrator). Each plan has at most one connected source of each kind. Teams
replace the plan's frozen roster, not the shared library. Initiatives replace
that plan's initiative inputs. Saved agreements remain unchanged.

Review mode is the default. Periodic checks capture changed content without
applying it. Automatic mode applies eligible changes only when the plan still
matches the source checkpoint; removals and local divergence require review.
Checks default to 15 minutes, configurable from 5 to 1440 minutes. Pausing stops
periodic checks; disconnecting retains the recorded history.

Capture records include table content and validation diagnostics. Protect the
Conway database and backups as planning data. These records are observations
made by Conway, not a mirror of Google's complete revision history. Google's
own [revision API documentation](https://developers.google.com/workspace/drive/api/guides/manage-revisions)
also cautions that API revision lists can omit older revisions.

A capture rejected because a required team was missing can become usable after
the roster is corrected. Explicitly review that capture again: Conway validates
it against the current plan before allowing application. The original capture's
diagnostics remain unchanged, and automatic checks do not bypass the review.

## Troubleshooting

| Symptom | Action |
|---|---|
| Connection unavailable | Check the environment variable, readable credentials file and startup log, then restart after correcting configuration. |
| Access denied or sheet not found | Confirm the service-account email has viewer access and the spreadsheet URL identifies the intended file. |
| Range or table validation fails | Include the header row; use the workbook schema and inspect the captured version's diagnostics. |
| No new version after a check | Identical consecutive content is deduplicated. Changes between checks may never be observed. |
| Apply conflict | Preview the version against the current plan and review local changes before applying again. |
| Automatic update awaits review | Inspect removals, validation failures and local plan divergence; do not treat this as a missed successful apply. |

The requirements and decisions live in
[specification 023](../specs/023-linked-google-sheets.md).
