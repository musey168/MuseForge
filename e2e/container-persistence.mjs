// Called by test-container.sh before and after force-recreating the app.
import fs from 'node:fs';
const [phase,stateFile]=process.argv.slice(2);
const base=process.env.LOOPFORGE_TEST_URL;
if(!['prepare','verify'].includes(phase)||!base||!stateFile)throw new Error('Expected phase, protected state path and test URL');
let state;
async function request(method,path,body){
  const headers={Cookie:state.cookie,Origin:base};
  if(body!==undefined && !(body instanceof FormData))headers['Content-Type']='application/json';
  const res=await fetch(base+path,{method,headers,body:body===undefined?undefined:body instanceof FormData?body:JSON.stringify(body)});
  if(!res.ok)throw new Error(`${method} ${path}: HTTP ${res.status}`);
  return res;
}
if(phase==='prepare'){
  const login=await fetch(base+'/api/auth/password',{method:'POST',headers:{Origin:base,'Content-Type':'application/json'},body:JSON.stringify({login:process.env.LOOPFORGE_TEST_USER||'admin',password:process.env.LOOPFORGE_TEST_PASSWORD})});
  if(!login.ok)throw new Error('Persistence test login failed');
  state={cookie:login.headers.get('set-cookie').split(';')[0],sessionId:'e2e-container-persistence'};
  fs.writeFileSync(stateFile,JSON.stringify(state),{mode:0o600});fs.chmodSync(stateFile,0o600);
}else{
  state=JSON.parse(fs.readFileSync(stateFile,'utf8'));
  console.log('Container recreation preserved the login session.');
}
