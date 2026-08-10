<script>
  import { t } from './i18n.js'
  import { daemonOnline } from './daemon.js'

  // Show the user a simple state, not the daemon's raw "running (pid …)" line.
  // null = the GUI is still bringing the daemon up on a fresh launch.
  $: online = $daemonOnline === true
  $: status = online ? $t('ui.status.running') : ($daemonOnline === null ? $t('ui.status.connecting') : $t('ui.status.not_running'))
</script>

<div class="stage" class:online>
  <div class="halo"></div>

  <svg class="avatar" viewBox="0 0 200 200" role="img" aria-label="Pylon">
    <defs>
      <linearGradient id="ring" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%"  stop-color="var(--accent)" />
        <stop offset="100%" stop-color="var(--accent-2)" />
      </linearGradient>
      <radialGradient id="core" cx="40%" cy="35%" r="75%">
        <stop offset="0%"  stop-color="#eaf1ff" />
        <stop offset="35%" stop-color="var(--accent)" />
        <stop offset="100%" stop-color="#1b2440" />
      </radialGradient>
      <radialGradient id="coreGlow" cx="50%" cy="50%" r="50%">
        <stop offset="0%"  stop-color="var(--accent-2)" stop-opacity="0.55" />
        <stop offset="100%" stop-color="var(--accent-2)" stop-opacity="0" />
      </radialGradient>
    </defs>

    <!-- outer dashed ring, slow spin -->
    <circle class="ring outer" cx="100" cy="100" r="86"
            fill="none" stroke="url(#ring)" stroke-width="2"
            stroke-dasharray="4 14" stroke-linecap="round" opacity="0.7" />

    <!-- middle ring, counter-spin -->
    <circle class="ring middle" cx="100" cy="100" r="66"
            fill="none" stroke="url(#ring)" stroke-width="3"
            stroke-dasharray="60 220" stroke-linecap="round" opacity="0.85" />

    <!-- orbiting nodes -->
    <g class="orbit">
      <circle cx="100" cy="34" r="4" fill="var(--accent-2)" />
      <circle cx="100" cy="166" r="3" fill="var(--accent)" />
    </g>

    <!-- soft core glow -->
    <circle class="glow" cx="100" cy="100" r="52" fill="url(#coreGlow)" />

    <!-- breathing core -->
    <circle class="corebody" cx="100" cy="100" r="40" fill="url(#core)" />
    <!-- specular highlight -->
    <ellipse cx="86" cy="84" rx="16" ry="11" fill="#ffffff" opacity="0.28" />
  </svg>

  <div class="name">Pylon</div>
  <div class="status" class:off={!online}>
    <span class="pip"></span>{status}
  </div>
</div>

<style>
  .stage {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
  }

  .halo {
    position: absolute;
    top: -30px;
    width: 260px; height: 260px;
    border-radius: 50%;
    background: radial-gradient(circle, var(--accent-glow), transparent 62%);
    filter: blur(14px);
    opacity: 0;
    transition: opacity 600ms var(--ease);
    pointer-events: none;
  }
  .stage.online .halo { opacity: 0.5; animation: breathe 5s ease-in-out infinite; }

  .avatar {
    width: 220px; height: 220px;
    overflow: visible;
    filter: grayscale(0.75) brightness(0.7);
    transition: filter 600ms var(--ease);
  }
  .stage.online .avatar { filter: none; }

  .ring, .orbit { transform-origin: 100px 100px; }
  .stage.online .ring.outer  { animation: spin 26s linear infinite; }
  .stage.online .ring.middle { animation: spin 16s linear infinite reverse; }
  .stage.online .orbit       { animation: spin 9s linear infinite; }

  .corebody, .glow { transform-origin: 100px 100px; }
  .stage.online .corebody { animation: pulse 4.5s ease-in-out infinite; }
  .stage.online .glow     { animation: pulse 4.5s ease-in-out infinite; }

  @keyframes spin   { to { transform: rotate(360deg); } }
  @keyframes pulse  { 0%,100% { transform: scale(1); } 50% { transform: scale(1.06); } }
  @keyframes breathe{ 0%,100% { opacity: 0.4; transform: scale(0.96); } 50% { opacity: 0.6; transform: scale(1.04); } }

  .name {
    font-size: 26px; font-weight: 800; letter-spacing: 0.5px;
    background: linear-gradient(120deg, var(--accent), var(--accent-2));
    -webkit-background-clip: text; background-clip: text;
    -webkit-text-fill-color: transparent;
  }

  .status {
    display: inline-flex; align-items: center; gap: 8px;
    font-size: 12px; color: var(--text-2);
    padding: 5px 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 999px;
    max-width: 260px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .pip {
    width: 7px; height: 7px; border-radius: 50%;
    background: var(--ok); box-shadow: 0 0 8px var(--ok);
    flex: 0 0 auto;
  }
  .status.off { color: var(--text-3); }
  .status.off .pip { background: var(--err); box-shadow: 0 0 8px var(--err); }
</style>
