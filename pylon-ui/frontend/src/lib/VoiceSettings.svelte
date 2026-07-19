<script>
  // Voice settings: pick the push-to-talk shortcut and get the exact way to bind
  // it on your system. A desktop app can't register a global hotkey itself on
  // most platforms, so the real binding lives in the OS / window manager — this
  // card captures the combo and shows the matching snippet (hyprland, Sway,
  // GNOME, KDE, macOS, Windows) that runs `pylon listen`.
  import { onMount } from 'svelte'
  import { Platform } from '../../wailsjs/go/main/App.js'

  const KEY = 'pylon.voice.hotkey'

  // Default matches the config's informational hotkey (super+p).
  let mods = ['SUPER']
  let key = 'P'
  let platform = 'hyprland'
  let capturing = false
  let copied = false

  const PLATFORMS = [
    { id: 'hyprland', label: 'Hyprland' },
    { id: 'sway', label: 'Sway / i3' },
    { id: 'gnome', label: 'GNOME' },
    { id: 'kde', label: 'KDE Plasma' },
    { id: 'linux', label: 'Diğer Linux' },
    { id: 'macos', label: 'macOS' },
    { id: 'windows', label: 'Windows' },
  ]

  onMount(async () => {
    try {
      const saved = JSON.parse(localStorage.getItem(KEY) || 'null')
      if (saved?.key) { mods = saved.mods || []; key = saved.key }
      if (saved?.platform) platform = saved.platform
      else platform = (await Platform()) || 'hyprland'
    } catch {}
  })

  function save() {
    localStorage.setItem(KEY, JSON.stringify({ mods, key, platform }))
  }

  // hyprland spells the main key with these names for non-character keys.
  const NAMED = { ' ': 'space', Enter: 'return', Escape: 'escape', Tab: 'tab', Backspace: 'backspace' }

  function onKey(e) {
    if (!capturing) return
    e.preventDefault()
    if (e.key === 'Escape') { capturing = false; return }
    if (['Control', 'Shift', 'Alt', 'Meta'].includes(e.key)) return // wait for the real key

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

  const cap = (x) => x[0] + x.slice(1).toLowerCase()
  $: pretty = [...mods.map(cap), key].join(' + ')

  // Per-platform binding. `snippet` is the line to paste (or the command to
  // bind); `note` says where it goes.
  function bindFor(p, mods, key) {
    const kl = key.toLowerCase()
    switch (p) {
      case 'hyprland':
        return { snippet: `bind = ${mods.join(' ')}, ${key}, exec, pylon listen`,
                 note: 'Bu satırı hyprland.conf dosyana ekle.' }
      case 'sway': {
        const map = { SUPER: 'Mod4', CTRL: 'Ctrl', ALT: 'Mod1', SHIFT: 'Shift' }
        const combo = [...mods.map((x) => map[x]), kl].join('+')
        return { snippet: `bindsym ${combo} exec pylon listen`,
                 note: '~/.config/sway/config (veya i3 config) dosyana ekle.' }
      }
      case 'gnome':
        return { snippet: 'pylon listen',
                 note: `Ayarlar → Klavye → Özel Kısayollar: komut “pylon listen”, kısayol ${pretty}.` }
      case 'kde':
        return { snippet: 'pylon listen',
                 note: `Sistem Ayarları → Kısayollar → Özel: komut “pylon listen”, kısayol ${pretty}.` }
      case 'macos': {
        const map = { SUPER: 'cmd', CTRL: 'ctrl', ALT: 'alt', SHIFT: 'shift' }
        const m = mods.map((x) => map[x])
        const lhs = m.length ? `${m.join(' + ')} - ${kl}` : kl
        return { snippet: `${lhs} : pylon listen`,
                 note: 'skhd ile (brew install skhd): ~/.config/skhd/skhdrc dosyasına ekle.' }
      }
      case 'windows': {
        const map = { SUPER: '#', CTRL: '^', ALT: '!', SHIFT: '+' }
        const sym = mods.map((x) => map[x]).join('')
        return { snippet: `${sym}${kl}::Run "pylon listen"`,
                 note: 'AutoHotkey v2 script (.ahk) olarak kaydet ve başlat.' }
      }
      default:
        return { snippet: 'pylon listen',
                 note: `Masaüstünün klavye kısayolları ayarında bu komutu ${pretty} tuşuna bağla.` }
    }
  }

  $: bind = bindFor(platform, mods, key)

  function onPlatform(e) { platform = e.target.value; save() }

  async function copy() {
    try {
      await navigator.clipboard.writeText(bind.snippet)
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
    Pylon ile konuşmak için bir kısayol ata. Uygulama global kısayolu kendi
    yakalayamaz — işletim sistemine göre aşağıdaki bağlamayı ekle, tuşa basınca
    Pylon dinlemeye başlar (<code>pylon listen</code>).
  </p>

  <div class="row">
    <span class="lbl">Kısayol</span>
    <button class="combo" class:capturing on:click={() => (capturing = !capturing)}>
      {#if capturing}tuşa bas…{:else}{pretty}{/if}
    </button>
  </div>

  <div class="row">
    <span class="lbl">Sistem</span>
    <select class="sel" value={platform} on:change={onPlatform}>
      {#each PLATFORMS as p}
        <option value={p.id}>{p.label}</option>
      {/each}
    </select>
  </div>

  <div class="bind">
    <code>{bind.snippet}</code>
    <button class="copy" on:click={copy}>{copied ? 'Kopyalandı ✓' : 'Kopyala'}</button>
  </div>
  <p class="note">{bind.note}</p>
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
  .combo, .sel {
    min-width: 150px;
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
  .note { margin: 10px 2px 0; font-size: 12px; line-height: 1.45; color: var(--text-2); }
</style>
