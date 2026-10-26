if (location.origin !== 'https://music.youtube.com') return {available:false};
const rgba = value => {
  const parts = value.match(/[\d.]+/g)?.map(Number) ?? [];
  return [parts[0]??0,parts[1]??0,parts[2]??0,parts[3]??1];
};
const blend = (top,bottom) => top.slice(0,3).map((v,i)=>v*top[3]+bottom[i]*(1-top[3]));
const bg = node => {
  const ancestors=[];
  for(let p=node;p;p=p.parentElement) ancestors.push(p);
  return ancestors.reverse().reduce((base,p)=>blend(rgba(getComputedStyle(p).backgroundColor),base),[255,255,255]);
};
const luminance = rgb => rgb.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((s,v,i)=>s+v*[.2126,.7152,.0722][i],0);
const visible = n => {
  const r=n.getBoundingClientRect(),s=getComputedStyle(n);
  return r.width>0&&r.height>0&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth&&s.visibility==='visible'&&s.opacity!=='0'&&!n.closest('[hidden],[aria-hidden="true"]');
};
const targets=[...document.querySelectorAll('button,a[role="button"],ytmusic-chip-cloud-chip-renderer a,[role="tab"],tp-yt-paper-tab,ytmusic-menu-service-item-renderer,ytmusic-menu-navigation-item-renderer,ytmusic-description-shelf-renderer,ytmusic-message-renderer')].filter(n=>visible(n)&&!n.closest('ytmusic-player,.ytp-player-content'));
const controls=targets.map(n=>{
  const leaves=[...n.querySelectorAll('yt-formatted-string,yt-attributed-string,.yt-core-attributed-string,span')].filter(e=>e.textContent.trim()&&!e.children.length&&visible(e));
  const text=leaves[0]??n,back=bg(text),front=blend(rgba(getComputedStyle(text).color),back);
  const l1=luminance(front),l2=luminance(back),r=n.getBoundingClientRect();
  return {tag:n.tagName,classes:String(n.className),label:(n.getAttribute('aria-label')||n.textContent.trim()).slice(0,65),fg:front.map(Math.round),bg:back.map(Math.round),contrast:+((Math.max(l1,l2)+.05)/(Math.min(l1,l2)+.05)).toFixed(2),disabled:n.matches(':disabled,[aria-disabled="true"]')||!!n.closest('[disabled]'),selected:n.matches('[aria-selected="true"],.iron-selected')||!!n.closest('[selected],[is-selected]'),rect:{x:Math.round(r.x),y:Math.round(r.y),w:Math.round(r.width),h:Math.round(r.height)}};
});
return {available:true,path:location.pathname,theme:document.documentElement.dataset.ytmgoTheme,viewport:{width:innerWidth,height:innerHeight},horizontalOverflow:document.documentElement.scrollWidth>innerWidth,controls,lowContrast:controls.filter(c=>c.label&&!c.disabled&&c.contrast<4.5)};
