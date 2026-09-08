// Explicitly await asynchronous predicates before deciding whether to poll again.
// A deadline also bounds a predicate whose network request never settles.
export async function waitForAsyncFunction(page,predicate,arg,{timeout=10000,polling=100}={}){
 const deadline=Date.now()+timeout;
 let timer;
 try{
  for(;;){
   const remaining=deadline-Date.now();
   if(remaining<=0)throw Error('Timed out waiting for asynchronous browser condition');
   const value=await Promise.race([page.evaluate(predicate,arg),new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Timed out waiting for asynchronous browser condition')),remaining);})]);
   clearTimeout(timer);
   if(value)return value;
   await new Promise(resolve=>setTimeout(resolve,Math.min(polling,Math.max(0,deadline-Date.now()))));
  }
 }finally{clearTimeout(timer);}
}
