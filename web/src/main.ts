import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { loadConfig } from './lib/config'

const target = document.getElementById('app')
if (!target) throw new Error('#app not found')

// 런타임 설정을 먼저 읽는다 — API base·Jira URL·기능 플래그가 첫 렌더 전에 확정돼야 한다.
// top-level await 은 빌드 타깃(es2020)에서 못 쓰므로 async IIFE 로 감싼다.
// index.html 의 인라인 부트 셸이 그동안 화면을 채우고 있어 흰 플래시는 없다.
void (async () => {
  await loadConfig()

  // 부트 셸을 비우고 그 자리에 마운트한다.
  target.innerHTML = ''
  mount(App, { target })
})()
