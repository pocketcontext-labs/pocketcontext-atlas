#!/usr/bin/env node
// Invoked only by the opt-in synthetic Go fixture TestMCPOAuthBrowser; no
// external OAuth providers, real projects or real data.
import assert from 'node:assert/strict';
import {createHash, randomBytes} from 'node:crypto';
import {mkdirSync} from 'node:fs';
import {createRequire} from 'node:module';
import {isAbsolute, join} from 'node:path';
import {fileURLToPath} from 'node:url';
const {chromium}=createRequire(fileURLToPath(new URL('../ui/package.json',import.meta.url)))('playwright');
const origin='https://knowledge.example.com';
const signIn=process.env.POCKETCONTEXT_SYNTHETIC_SIGNIN,port=process.env.POCKETCONTEXT_SYNTHETIC_PORT;
assert(signIn?.startsWith(origin+'/bundle/login#') && /^\d+$/.test(port),'requires synthetic Go fixture');
const shots=process.env.POCKETCONTEXT_OAUTH_SCREENSHOTS||'';
if(shots){assert(isAbsolute(shots),'screenshot folder must be absolute');mkdirSync(shots,{recursive:true})}
const browser=await chromium.launch({args:['--no-proxy-server',`--host-resolver-rules=MAP knowledge.example.com 127.0.0.1:${port}`]});
const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,ignoreHTTPSErrors:true});
const problems=[];
await context.route('**/*',async route=>{
 const u=new URL(route.request().url());
 if(u.origin===origin) await route.continue();
 else if(u.origin==='https://client.example.com' && u.pathname==='/callback') await route.fulfill({status:200,contentType:'text/html',body:'<!doctype html><title>Synthetic callback</title>'});
 else {problems.push('unexpected external request');await route.abort()}
});
// Every width fits without horizontal scrolling. On the server-rendered OAuth
// pages no visible text is under 16px; the reader's own login keeps its styles.
async function checkLayout(page,name,fonts=true){
 for(const width of [390,320]){
  await page.setViewportSize({width,height:844});
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${name} must fit ${width}px`);
  const small=!fonts?[]:await page.evaluate(()=>[...document.body.querySelectorAll('*')].filter(e=>e.offsetParent!==null&&[...e.childNodes].some(n=>n.nodeType===3&&n.textContent.trim())&&parseFloat(getComputedStyle(e).fontSize)<16).map(e=>e.tagName+': '+e.textContent.trim().slice(0,40)));
  assert.deepEqual(small,[],`${name} text under 16px at ${width}px`);
  if(shots)await page.screenshot({path:join(shots,`${name}-${width}.png`),fullPage:true});
 }
 await page.setViewportSize({width:390,height:844});
}
try{
 const reader=await context.newPage();reader.on('pageerror',e=>problems.push(e.message));
 await reader.goto(origin+'/oauth/connections');
 // Atlas has no connected-apps view, so Core's page stays, in Atlas's style.
 assert.equal(reader.url(),origin+'/oauth/connections');
 await reader.getByText('Sign in to the reader in this browser first, then return here.',{exact:true}).waitFor();
 const call=async(path,body,token='',form=false)=>reader.evaluate(async({path,body,token,form})=>{
  const response=await fetch(path,{method:'POST',headers:{'Content-Type':form?'application/x-www-form-urlencoded':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:form?new URLSearchParams(body).toString():JSON.stringify(body)});
  return {status:response.status,data:await response.json()};
 },{path,body,token,form});
 const registered=await call('/oauth/register',{client_name:'Synthetic <script>bad()</script> client',redirect_uris:['https://first.example.com/callback','https://client.example.com/callback'],token_endpoint_auth_method:'none'});
 assert.equal(registered.status,201);
 const verifier=randomBytes(32).toString('base64url');const client=registered.data.client_id;
 const params=new URLSearchParams({client_id:client,redirect_uri:'https://client.example.com/callback',response_type:'code',code_challenge_method:'S256',code_challenge:createHash('sha256').update(verifier).digest('base64url'),resource:origin+'/mcp',scope:'knowledge',state:'synthetic-state'});
 const consent=await context.newPage();consent.on('pageerror',e=>problems.push(e.message));
 await consent.goto(origin+'/oauth/authorize?'+params);
 await consent.getByText('Registered callback: https://client.example.com/callback',{exact:true}).waitFor();
 await consent.getByText('The app name is supplied by its developer. Check that you intended to connect it.',{exact:true}).waitFor();
 await consent.getByText('is asking to read and change your knowledge in every space, including originals, citations and published pages.',{exact:false}).waitFor();
 await consent.getByText('This does not grant server administration or shell access.',{exact:true}).waitFor();
 assert.equal(await consent.locator('strong').first().textContent(),'Synthetic <script>bad()</script> client');
 assert.equal(await consent.locator('script').count(),1);
 assert.equal(await consent.locator('svg').count(),1,'inline logo');
 assert.equal(await consent.locator('.product-brand').textContent(),'PocketContext Atlas');
 const allow=consent.getByRole('button',{name:'Allow access',exact:true}),cancel=consent.getByRole('button',{name:'Cancel',exact:true}),login=consent.getByRole('link',{name:'Log in',exact:true});
 // Signed out: one Log in action replaces Allow and Cancel.
 await login.waitFor();assert.equal(await allow.count(),0);assert.equal(await cancel.count(),0);
 assert.equal(await login.getAttribute('href'),'/#/signin?next='+encodeURIComponent('/oauth/authorize?'+params));
 assert.equal(await consent.getByText('open the reader').count(),0);
 assert.equal(await consent.getByRole('link',{name:'Manage connected apps',exact:true}).getAttribute('href'),'/oauth/connections');
 await checkLayout(consent,'consent-signed-out');
 // A token that looks valid locally but the server rejects: Allow reveals Log in,
 // and the reader shows its form instead of bouncing back to consent.
 const part=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
 const stale=`${part({alg:'HS256'})}.${part({exp:4102444800,type:'auth',id:'synthetic',collectionId:'synthetic'})}.synthetic`;
 await consent.evaluate(token=>localStorage.setItem('pocketcontext.reader.auth',JSON.stringify({token,record:{id:'synthetic',collectionName:'users'}})),stale);
 await consent.reload();
 await allow.click();
 await consent.getByText('Your sign-in has expired. Log in to continue.',{exact:true}).waitFor();
 assert.equal(await allow.count(),0);
 await login.click();
 await consent.waitForURL(origin+'/#/signin?next=**');
 await consent.getByLabel('Password',{exact:true}).waitFor();
 assert.equal(await consent.evaluate(()=>localStorage.getItem('pocketcontext.reader.auth')||''),'','rejected token cleared');
 await checkLayout(consent,'signin-route',false);
 // The browser never learns or enters the synthetic account's password.
 await reader.goto(signIn);await reader.waitForURL(origin+'/');
 // Already signed in: the same route goes straight back to the request.
 await consent.reload();
 await consent.waitForURL(origin+'/oauth/authorize?**');
 assert.equal(consent.url(),origin+'/oauth/authorize?'+params);
 await allow.waitFor();await cancel.waitFor();assert.equal(await login.isVisible(),false);
 const box=await allow.boundingBox(),viewport=consent.viewportSize();
 assert(box.width>viewport.width/2,'Allow access is the primary full-width action');
 await checkLayout(consent,'consent-signed-in');
 await allow.click();
 await consent.waitForURL('https://client.example.com/callback?**');
 const callback=new URL(consent.url());assert.equal(callback.searchParams.get('state'),'synthetic-state');
 const tokens=await call('/oauth/token',{grant_type:'authorization_code',client_id:client,code:callback.searchParams.get('code'),redirect_uri:'https://client.example.com/callback',code_verifier:verifier,resource:origin+'/mcp'},'',true);
 assert.equal(tokens.status,200);
 assert.equal((await call('/mcp',{},tokens.data.access_token)).status,200);
 await reader.goto(origin+'/oauth/connections');
 await reader.getByRole('button',{name:'Revoke access',exact:true}).waitFor();
 await checkLayout(reader,'connections');
 await reader.getByRole('button',{name:'Revoke access',exact:true}).click();
 await reader.getByText('Access revoked.',{exact:true}).waitFor();
 assert.equal((await call('/mcp',{},tokens.data.access_token)).status,401);
 assert.equal((await call('/oauth/token',{grant_type:'refresh_token',client_id:client,refresh_token:tokens.data.refresh_token,resource:origin+'/mcp'},'',true)).status,400);
 // Capture Chromium's native-scheme navigation request. No OS handler is
 // installed by this fixture; native-client receipt remains a manual check.
 for(const redirect of ['com.example.app:/oauth/callback','cursor://anysphere.cursor-mcp/oauth/callback']){
  const registeredNative=await call('/oauth/register',{client_name:'Synthetic native client',redirect_uris:[redirect],token_endpoint_auth_method:'none'});
  assert.equal(registeredNative.status,201);
  const nativeVerifier=randomBytes(32).toString('base64url');
  const nativeParams=new URLSearchParams({client_id:registeredNative.data.client_id,redirect_uri:redirect,response_type:'code',code_challenge_method:'S256',code_challenge:createHash('sha256').update(nativeVerifier).digest('base64url'),scope:'knowledge',state:'native-state'});
  const native=await context.newPage();native.on('pageerror',e=>problems.push(e.message));
  await native.goto(origin+'/oauth/authorize?'+nativeParams);
  await native.getByText('Registered callback: '+redirect,{exact:true}).waitFor();
  const cdp=await context.newCDPSession(native);await cdp.send('Page.enable');
  let timer;
  const requested=new Promise((resolve,reject)=>{
   timer=setTimeout(()=>reject(new Error('native callback navigation was not requested')),10000);
   cdp.on('Page.frameRequestedNavigation',event=>{
    if(event.url.startsWith(redirect+'?')){clearTimeout(timer);resolve(event.url)}
   });
  });
  let nativeURL;
  try{await native.getByRole('button',{name:'Allow access',exact:true}).click();nativeURL=new URL(await requested)}
  finally{clearTimeout(timer);await cdp.detach();await native.close()}
  assert.equal(nativeURL.searchParams.get('state'),'native-state');
  assert(nativeURL.searchParams.get('code'));
  const nativeTokens=await call('/oauth/token',{grant_type:'authorization_code',client_id:registeredNative.data.client_id,code:nativeURL.searchParams.get('code'),redirect_uri:redirect,code_verifier:nativeVerifier},'',true);
  assert.equal(nativeTokens.status,200);
  assert.equal((await call('/mcp',{},nativeTokens.data.access_token)).status,200);
 }
 assert.deepEqual(problems,[]);
 console.log('ok - styled consent and connections at 390/320px, consent Log in, stale token recovery, reader link login without password, escaped client name, exact callback, OAuth exchange, authenticated MCP, browser revocation, private-scheme consent navigation');
}finally{await context.close();await browser.close()}
