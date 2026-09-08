import {waitForAsyncFunction} from './async-condition.mjs';
export async function chooseUpdate(page, title) {
  const select=page.locator('[data-announcement-select]');
  if(await select.count()) await select.selectOption({label:title});
}
export async function readAllUpdates(page) {
  const select=page.locator('[data-announcement-select]');
  if(await select.count()) {
    const values=await select.locator('option').evaluateAll(options=>options.map(o=>o.value));
    for(const value of values) await select.selectOption(value);
  }
  await waitForAsyncFunction(page,async()=>{
    const r=await fetch('/api/announcements',{headers:{Authorization:'Bearer '+localStorage.getItem('conway_token')}});
    return r.ok&&(await r.json()).features.every(f=>f.announced);
  });
}
