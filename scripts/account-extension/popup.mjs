import { transferSession, failureDescription } from './session-policy.mjs';
const messages = {
  en: ['Connect to ytmgo', 'Select the account you want to use in the current YouTube Music tab.', 'I confirm that this page has the account I want to connect.', 'When you click, this extension sends only the YouTube session and selected account index directly to ytmgo on this computer. It never saves credentials or transfers them automatically.', 'Connect account', 'Keep this panel open during transfer. Do not switch tabs or accounts.', 'Connecting. Keep this panel open.', 'ytmgo received the session. Return to the app to finish verification.', 'Could not connect. Reopen Sign In in ytmgo, check the selected Music account, then reopen this panel and retry.'],
  'zh-Hans': ['连接到 ytmgo', '先在当前 YouTube Music 标签页中选中要使用的账号。', '我已确认当前页面选中的是要连接的账号。', '点击后仅把 YouTube 登录会话和当前账号索引直接交给本机 ytmgo。本扩展不保存登录信息，也不会自动读取或传送。', '连接账号', '传送期间请保持本面板打开，不要切换账号或标签页。', '正在连接，请保持本面板打开。', 'ytmgo 已接收会话，请返回应用等待账号验证完成。', '连接失败。请在 ytmgo 重新打开登录，确认 Music 当前账号，再重新打开本面板重试。'],
  'zh-Hant': ['連接到 ytmgo', '先在目前 YouTube Music 分頁中選取要使用的帳號。', '我已確認目前頁面選取的是要連接的帳號。', '點擊後僅把 YouTube 登入工作階段與目前帳號索引直接交給本機 ytmgo。本擴充功能不儲存登入資料，也不會自動讀取或傳送。', '連接帳號', '傳送期間請保持此面板開啟，不要切換帳號或分頁。', '正在連接，請保持此面板開啟。', 'ytmgo 已收到工作階段，請返回應用程式等待帳號驗證完成。', '連接失敗。請在 ytmgo 重新開啟登入，確認 Music 目前帳號，再重新開啟此面板重試。'],
  ja: ['ytmgo に接続', '現在の YouTube Music タブで、使用するアカウントを選択してください。', 'このページで接続するアカウントが選択されていることを確認しました。', 'クリックすると、YouTube のセッションと選択したアカウント番号のみをこの Mac の ytmgo に直接渡します。拡張機能は認証情報を保存せず、自動転送もしません。', 'アカウントを接続', '転送中はこのパネルを開いたままにして、タブやアカウントを切り替えないでください。', '接続中です。このパネルを開いたままにしてください。', 'ytmgo が受信しました。アプリに戻り、アカウントの確認をお待ちください。', '接続できませんでした。ytmgo でログインを開き直し、Music のアカウントを確認して、このパネルから再試行してください。'],
};
const lang = navigator.language.toLowerCase();
const key = lang.startsWith('ja') ? 'ja' : /zh-(tw|hk|mo|hant)/.test(lang) ? 'zh-Hant' : lang.startsWith('zh') ? 'zh-Hans' : 'en';
const text = messages[key];
document.documentElement.lang = key;
for (const [id, index] of [['title',0],['intro',1],['confirm-text',2],['permissions',3],['transfer',4],['status',5]]) document.getElementById(id).textContent = text[index];
const confirmation = document.getElementById('confirm-account');
const button = document.getElementById('transfer');
const status = document.getElementById('status');
let attempted = false;
confirmation.checked = false;
confirmation.addEventListener('change', () => { button.disabled = attempted || !confirmation.checked; });
button.addEventListener('click', async () => {
  if (attempted || !confirmation.checked) return;
  attempted = true;
  button.disabled = true;
  confirmation.disabled = true;
  status.textContent = text[6];
  try {
    await transferSession(chrome, fetch);
    status.textContent = text[7];
  } catch (error) {
    const safe = failureDescription(error);
    status.textContent = text[8] + ' (' + safe.stage + ' / ' + safe.category + ')';
  }
});
