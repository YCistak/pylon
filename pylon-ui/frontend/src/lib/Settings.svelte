<script>
  import { AVAILABLE, layout } from './widgets.js'

  function toggle(id, on) {
    if (on) layout.enable(id, 'left')
    else layout.disable(id)
  }
</script>

<div class="settings">
  <header class="head">
    <h2>Ayarlar</h2>
    <p class="sub">Widget'ları buradan aç/kapat ve hangi sütunda görüneceğini seç.
      Ana sayfa yalnızca burada açtıklarını gösterir.</p>
  </header>

  <section class="card">
    <h3>Widget'lar</h3>
    <ul class="list">
      {#each AVAILABLE as w}
        {@const col = $layout[w.id]}
        {@const on = !!col}
        <li class:on>
          <span class="tile" style="--wa: {w.accent}">{w.icon}</span>
          <span class="name">{w.title}</span>

          {#if on}
            <div class="cols">
              <button class:active={col === 'left'}  on:click={() => layout.setColumn(w.id, 'left')}>Sol</button>
              <button class:active={col === 'right'} on:click={() => layout.setColumn(w.id, 'right')}>Sağ</button>
            </div>
          {/if}

          <button class="switch" class:on aria-label="aç/kapat" on:click={() => toggle(w.id, !on)}>
            <span class="knob"></span>
          </button>
        </li>
      {/each}
    </ul>
  </section>

  <p class="note">Parola ekleme, servis aç/kapa ve ses ayarları — 4. adımda gelecek.</p>
</div>

<style>
  .settings { flex: 1; padding: 40px 44px; overflow: auto; max-width: 720px; }
  .head h2 { margin: 0 0 6px; color: var(--text-0); font-size: 24px; }
  .sub { color: var(--text-2); line-height: 1.55; margin: 0 0 26px; max-width: 520px; }

  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    padding: 8px 20px 16px;
  }
  .card h3 { color: var(--text-1); font-size: 14px; margin: 16px 4px 6px; }

  .list { list-style: none; margin: 0; padding: 0; }
  .list li {
    display: flex; align-items: center; gap: 14px;
    padding: 14px 4px;
    border-top: 1px solid var(--border);
  }
  .list li:first-child { border-top: none; }

  .tile {
    width: 34px; height: 34px; flex: 0 0 auto;
    display: grid; place-items: center; font-size: 16px;
    border-radius: 10px;
    background: color-mix(in srgb, var(--wa) 18%, transparent);
    border: 1px solid color-mix(in srgb, var(--wa) 30%, transparent);
  }
  .name { flex: 1; color: var(--text-1); font-weight: 700; }

  .cols { display: flex; gap: 4px; background: var(--bg-2); padding: 3px; border-radius: 9px; }
  .cols button {
    border: none; background: transparent; color: var(--text-2);
    font-size: 12px; font-weight: 600; padding: 4px 10px;
    border-radius: 7px; cursor: pointer;
    transition: background var(--dur), color var(--dur);
  }
  .cols button.active { background: var(--surface-2); color: var(--text-0); }

  .switch {
    width: 42px; height: 24px; flex: 0 0 auto;
    border: 1px solid var(--border-2); border-radius: 999px;
    background: var(--bg-2); cursor: pointer; padding: 0;
    position: relative; transition: background var(--dur), border-color var(--dur);
  }
  .switch.on { background: linear-gradient(120deg, var(--accent), var(--accent-2)); border-color: transparent; }
  .knob {
    position: absolute; top: 2px; left: 2px;
    width: 18px; height: 18px; border-radius: 50%;
    background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,.4);
    transition: transform var(--dur) var(--ease);
  }
  .switch.on .knob { transform: translateX(18px); }

  .note { color: var(--text-3); font-size: 12px; margin-top: 22px; }
</style>
