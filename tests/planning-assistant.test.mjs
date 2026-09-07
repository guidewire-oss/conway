import test from 'node:test';
import assert from 'node:assert/strict';
import {assistantAnswerHTML,safeAssistantSource} from '../app/js/planning-assistant.js';

test('assistant evidence escapes source text and refuses arbitrary links',()=>{
 const html=assistantAnswerHTML({context:{planId:'plan-a',planName:'<script>bad</script>'},summary:'<img src=x onerror=alert(1)>',facts:[{kind:'Modeled',title:'<b>Beacon</b>',details:['<svg onload=alert(1)>'],source:{label:'Open',url:'javascript:alert(1)'}}],gaps:[]});
 assert.ok(!html.includes('<script>'));assert.ok(!html.includes('<img'));assert.ok(!html.includes('href="javascript:'));assert.ok(html.includes('&lt;b&gt;Beacon'));
});
test('assistant links stay in the authorized plan and existing workflows',()=>{
 assert.equal(safeAssistantSource('?view=plan&plan=plan-a&planView=timeline','plan-a'),true);
 for(const url of ['https://elsewhere.test/','//elsewhere.test/','?view=plan&plan=plan-b&planView=timeline','?view=plan&plan=plan-a&planView=unknown','javascript:alert(1)'])assert.equal(safeAssistantSource(url,'plan-a'),false);
});
test('assistant displays explicit context and unknowns even for an empty answer',()=>{
 const html=assistantAnswerHTML({context:{planId:'p',planName:'Atlas',planFingerprint:'revision',periodStart:'',team:'Atlas'},summary:'Manual review',facts:[],gaps:['Missing capture']});
 assert.match(html,/Missing capture/);assert.match(html,/Saved plan/);assert.match(html,/relative weeks/i);assert.match(html,/Atlas/);
});
