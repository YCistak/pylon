// Official brand marks. GitHub/Spotify/FreshRSS: Simple Icons project
// (github.com/simple-icons/simple-icons — accurate single-color reproductions).
// `fill="currentColor"` on the <svg> root lets each inherit the widget's per-service
// accent color via CSS, matching how the widget tints its icon — see
// Widget.svelte/Settings.svelte (`{@html icon}`, these strings are static/
// developer-authored, never user input).
//
// Google Calendar/Drive: those brands are inherently multi-color and gradient — a
// flattened single-tone recreation doesn't read as the real icon, so these two are
// bundled as full-color SVGs of Google's current (2026 gradient redesign) marks:
// the bulbous green/blue/yellow Drive triangle and the blue "31" Calendar. Kept as
// separate <img>-referenced files (not inlined) so their internal gradient/mask ids
// can't collide with each other on the page.
//
// Usage follows each brand's guidelines for indicating integration ("connects to X"):
// unmodified mark, no implied endorsement.

import googleCalendarSvg from '../assets/icons/google-calendar.svg'
import googleDriveSvg from '../assets/icons/google-drive.svg'

export const iconGoogleCalendar = `<img src="${googleCalendarSvg}" alt="Google Calendar" />`

export const iconGoogleDrive = `<img src="${googleDriveSvg}" alt="Google Drive" />`

export const iconGitHub = `<svg role="img" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>GitHub</title><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>`

export const iconSpotify = `<svg role="img" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>Spotify</title><path d="M12 0C5.4 0 0 5.4 0 12s5.4 12 12 12 12-5.4 12-12S18.66 0 12 0zm5.521 17.34c-.24.359-.66.48-1.021.24-2.82-1.74-6.36-2.101-10.561-1.141-.418.122-.779-.179-.899-.539-.12-.421.18-.78.54-.9 4.56-1.021 8.52-.6 11.64 1.32.42.18.479.659.301 1.02zm1.44-3.3c-.301.42-.841.6-1.262.3-3.239-1.98-8.159-2.58-11.939-1.38-.479.12-1.02-.12-1.14-.6-.12-.48.12-1.021.6-1.141C9.6 9.9 15 10.561 18.72 12.84c.361.181.54.78.241 1.2zm.12-3.36C15.24 8.4 8.82 8.16 5.16 9.301c-.6.179-1.2-.181-1.38-.721-.18-.601.18-1.2.72-1.381 4.26-1.26 11.28-1.02 15.721 1.621.539.3.719 1.02.419 1.56-.299.421-1.02.599-1.559.3z"/></svg>`

export const iconFreshRSS = `<svg role="img" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>FreshRSS</title><path d="M11.738.003C5.217.151.006 5.476 0 11.999h2.25a9.74 9.74 0 0 1 6.02-9.008 9.74 9.74 0 0 1 10.628 2.113 9.74 9.74 0 0 1 2.113 10.626 9.74 9.74 0 0 1-9.01 6.02V24c4.85 0 9.23-2.927 11.088-7.408A12 12 0 0 0 11.738.003m.264.5v1.252c-1.32 0-2.653.25-3.922.775-3.674 1.521-6.06 5.03-6.256 8.97H.574c.2-4.443 2.89-8.413 7.028-10.126A11.4 11.4 0 0 1 12 .503m-.031 3.434a8 8 0 0 0-3.055.613A8.07 8.07 0 0 0 3.938 12h2.25a5.8 5.8 0 0 1 3.589-5.37 5.81 5.81 0 0 1 6.334 1.26 5.8 5.8 0 0 1 1.26 6.335 5.8 5.8 0 0 1-5.37 3.588v2.25a8.07 8.07 0 0 0 7.451-4.977 8.07 8.07 0 0 0-1.75-8.788c-2.125-2.125-4.667-2.365-5.732-2.362m.03.501V5.69a6.3 6.3 0 0 0-2.415.477c-2.2.911-3.633 2.987-3.823 5.332h-1.25C4.703 8.65 6.44 6.115 9.105 5.012A7.5 7.5 0 0 1 12 4.438M18.312 12h1.248a7.6 7.6 0 0 1-.57 2.896c-1.104 2.664-3.639 4.4-6.488 4.593v-1.25c2.345-.19 4.42-1.621 5.333-3.822A6.3 6.3 0 0 0 18.312 12m3.936 0h1.248c0 1.483-.278 2.978-.867 4.4-1.714 4.137-5.685 6.828-10.127 7.027v-1.25c3.94-.197 7.45-2.582 8.97-6.254A10.3 10.3 0 0 0 22.249 12m-7.155 0A3.094 3.094 0 0 1 12 15.094 3.094 3.094 0 0 1 8.906 12 3.094 3.094 0 0 1 12 8.906 3.094 3.094 0 0 1 15.094 12"/></svg>`
