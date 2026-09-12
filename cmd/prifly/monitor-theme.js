'use strict';
// Run before the stylesheet so a saved theme also applies to the first paint.
(() => {
 const key = 'prifly.monitor.theme';
 const system = window.matchMedia('(prefers-color-scheme: dark)');
 let preference = 'system';
 try {
  const saved = localStorage.getItem(key);
  if(['light', 'dark', 'system'].includes(saved)) preference = saved;
 } catch { /* A blocked browser store must not prevent theme selection. */ }
 const apply = () => {
  document.documentElement.dataset.theme = preference === 'system' ? (system.matches ? 'dark' : 'light') : preference;
 };
 apply();
 system.addEventListener('change', apply);
 document.addEventListener('DOMContentLoaded', () => {
  const select = document.getElementById('theme');
  select.value = preference;
  select.addEventListener('change', () => {
   preference = select.value;
   apply();
   try { localStorage.setItem(key, preference); } catch { /* Keep the session choice. */ }
  });
 });
})();
