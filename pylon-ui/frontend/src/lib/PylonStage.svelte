<script>
  import { onMount, onDestroy } from 'svelte'
  import { Status } from '../../wailsjs/go/main/App.js'

  let status = '…'
  let online = false
  let timer

  async function refresh() {
    try { status = await Status(); online = true }
    catch (e) { status = 'daemon kapalı'; online = false }
  }
  onMount(() => { refresh(); timer = setInterval(refresh, 5000) })
  onDestroy(() => clearInterval(timer))
</script>

<div class="stage">
  <div class="orb {online ? 'live' : 'idle'}">
    <div class="core"></div>
  </div>
  <div class="name">Pylon</div>
  <div class="status">{status}</div>
</div>

<style>
  .stage { display: flex; flex-direction: column; align-items: center; gap: 14px; }
  .orb {
    width: 150px; height: 150px;
    border-radius: 50%;
    display: grid; place-items: center;
    background: radial-gradient(circle at 35% 30%, #1b3a5c, #0d1320 70%);
    border: 1px solid #244b6e;
  }
  .orb .core {
    width: 70px; height: 70px;
    border-radius: 50%;
    background: radial-gradient(circle at 40% 35%, #8be9ff, #3b82f6 70%);
  }
  .orb.live { box-shadow: 0 0 40px rgba(59,130,246,.45); animation: pulse 3.2s ease-in-out infinite; }
  .orb.idle { filter: grayscale(.7) brightness(.8); }
  @keyframes pulse {
    0%, 100% { box-shadow: 0 0 30px rgba(59,130,246,.35); }
    50%      { box-shadow: 0 0 55px rgba(110,231,255,.6); }
  }
  .name { font-size: 22px; font-weight: 700; letter-spacing: .5px; color: #eaf2fb; }
  .status { font-size: 12px; color: #7d93a8; }
</style>
