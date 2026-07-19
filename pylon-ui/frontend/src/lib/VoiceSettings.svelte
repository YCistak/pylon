<script>
  // Voice settings: pick the push-to-talk shortcut and get the exact line to
  // paste into the compositor. On Wayland an app can't grab a global hotkey
  // itself, so the real binding lives in hyprland's config — this card captures
  // the combo and generates that `bind = …, exec, pylon listen` line.
  import { onMount } from 'svelte'

  const KEY = 'pylon.voice.hotkey'

  // Default matches the config's informational hotkey (super+p).
  let mods = ['SUPER']
  let key = 'P'
  let capturing = false
  let copied = false

  onMount(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(KEY) || 'null')
      if (saved?.key) { mods = saved.mods || []; key = saved.key }
    } catch {}
  })

  function save() {
    localStorage.setItem(KEY, JSON.stringify({ mods, key }))
  }

  // hyprland spells the main key with these names for non-character keys.
  const NAMED = { ' ': 'space', Enter: 'return', Escape: 'escape', Tab: 'tab', Backspace: 'backspace' }

  function onKey(e) {
    if (!capturing) return
    e.preventDefault()
    if (e.key === 'Escape') { capturing = false; return }
    // Ignore lone modifier presses — wait for the real key.
    if (['Control', 'Shift', 'Alt', 'Meta'].includes(e.key)) return

    const m = []
    if (e.metaKey) m.push('SUPER')
    if (e.ctrlKey) m.push('CTRL')
    if (e.altKey) m.push('ALT')
    if (e.shiftKey) m.push('SHIFT')

    let k = NAMED[e.key] || e.key
    if (k.length === 1) k = k.toUpperCase()

    mods = m
    key = k
    capturing = false
    save()
  }

  // "Super + P" for display; "SUPER, P" for the bind line.
  $: pretty = [...mods.map((x) => x[0] + x.slice(1).toLowerCase()), key].join(' + ')
  $: bindLine = `bind = ${mods.join(' ')}, ${key}, exec, pylon listen`

  async function copy() {
    try {
      await navigator.clipboard.writeText(bindLine)
      copied = true
      setTimeout(() => (copied = false), 1400)
    } catch {}
  }
</script>

<svelte:window on:keydown={onKey} />

<section class="card">
  <div class="card-head">
    <div class="card-title"><h3>Ses & Konuşma</h3></div>
  </div>
  <p class="hint">
    Pylon ile konuşmak için bir kısayol ata. Wayland'da uygulama kısayolu kendi
    yakalayamaz — aşağıdaki satırı <code>hyprland.conf</code>'a ekle, tuşa basınca
    Pylon dinlemeye başlar.
  </p>

  <div class="row">
    <span class="lbl">Konuşma kısayolu</span>
    <button class="combo" class:capturing on:click={() => (capturing = !capturing)}>
      {#if capturing}tuşa bas…{:else}{pretty}{/if}
    </button>
  </div>

  <div class="bind">
    <code>{bindLine}</code>
    <button class="copy" on:click={copy}>{copied ? 'Kopyalandı ✓' : 'Kopyala'}</button>
  </div>
</section>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    border-radius: 16px;
    padding: 18px 20px;
  }
  .card-head { margin-bottom: 6px; }
  .card-title h3 { margin: 0; font-size: 15px; color: var(--text-1); }
  .hint { margin: 0 0 14px; font-size: 13px; line-height: 1.5; color: var(--text-2); }
  .hint code { font-size: 12px; background: var(--surface-2); padding: 1px 5px; border-radius: 5px; }

  .row { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
  .lbl { font-size: 14px; color: var(--text-1); }
  .combo {
    min-width: 130px;
    padding: 8px 14px;
    border-radius: 10px;
    font: inherit; font-size: 13px; font-weight: 600;
    cursor: pointer;
    color: var(--text-1);
    background: var(--surface-2);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.12));
    transition: border-color 160ms, box-shadow 160ms;
  }
  .combo.capturing {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
    color: var(--accent);
  }

  .bind {
    display: flex; align-items: center; gap: 10px;
    padding: 10px 12px;
    border-radius: 10px;
    background: var(--bg-0, rgba(0, 0, 0, 0.35));
    border: 1px solid var(--border, rgba(255, 255, 255, 0.06));
  }
  .bind code {
    flex: 1; min-width: 0;
    font-family: ui-monospace, monospace; font-size: 12.5px;
    color: var(--text-1);
    overflow-x: auto; white-space: nowrap;
  }
  .copy {
    flex: none;
    padding: 6px 12px; border-radius: 8px;
    font: inherit; font-size: 12px; font-weight: 600; cursor: pointer;
    color: #0b0f17;
    background: linear-gradient(135deg, var(--accent), var(--accent-2));
    border: none;
  }
</style>
