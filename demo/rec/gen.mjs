// Builds a VHS tape from a step list and records, for every visible step, the
// time it happens in the recorded footage (hidden steps don't advance it).
// usage: node gen.mjs <take-name>
//   -> $DEMO_WORK/rec/<take>.tape, <take>.events.json, <take>.cut.json
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { takes } from './takes.mjs';

const W = process.env.DEMO_WORK ?? path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '.work');
const MOCK = process.env.MOCK_ADDR ?? '127.0.0.1:8788';
const name = process.argv[2];
const take = takes[name];
if (!take) throw new Error(`unknown take ${name}; one of ${Object.keys(takes).join(', ')}`);
const TYPE_MS = 55;
// Record in slow motion and play back faster: VHS captures a fixed number of
// frames per second, so this multiplies the playback frame rate.
const SLOW = take.slow ?? 1;

const theme = {
  name: 'pho', black: '#1e2021', red: '#ff7b72', green: '#3fb950', yellow: '#d29922', blue: '#58a6ff',
  magenta: '#bc8cff', cyan: '#39c5cf', white: '#b1bac4', brightBlack: '#6e7681', brightRed: '#ffa198',
  brightGreen: '#56d364', brightYellow: '#e3b341', brightBlue: '#79c0ff', brightMagenta: '#d2a8ff',
  brightCyan: '#56d4dd', brightWhite: '#ffffff', background: '#1e2021', foreground: '#e6edf3',
  selection: '#3b2f66', cursor: '#a78bfa',
};

const out = [
  `Output ${name}-frames/`,
  'Set Shell zsh',
  'Set FontFamily "JetBrains Mono"',
  "Set FontSize 18", "Set LineHeight 1.25",
  "Set Width 1920",
  "Set Height 1080",
  "Set Padding 22",
  'Set Framerate 15',
  `Set TypingSpeed ${TYPE_MS}ms`,
  `Set Theme ${JSON.stringify(theme)}`,
  'Hide',
  `Type "export PATH=${W}/bin:$PATH PHO_SLOWMO=${SLOW} PHO_MOCK_API=http://${MOCK} XDG_CACHE_HOME=${W}/xdg/c XDG_STATE_HOME=${W}/xdg/s XDG_DATA_HOME=${W}/xdg/d; cd ${W}/world/repos"`,
  'Enter',
  `Type "alias pho='script -q -r ${W}/rec/${name}.keys.rec pho'"`,
  'Enter',
  `Type "PROMPT='%F{8}~/code%f %F{141}❯%f '; clear"`,
  'Enter',
  'Sleep 600ms',
];

let t = 0;          // visible footage time, seconds
let n = 0;          // visible keystrokes so far (VHS adds a little latency to each)
let p = 0;          // keystrokes delivered to pho itself (matches script -r input records)
let hidden = true;
const events = [];  // {t, key} keycap events, {t, mark} scene markers
const vis = (s) => { if (!hidden) t += s; };

const vhsKey = { enter: 'Enter', esc: 'Escape', tab: 'Tab', space: 'Space', 'ctrl+p': 'Ctrl+P', 'ctrl+d': 'Ctrl+D',
  'ctrl+s': 'Ctrl+S', 'ctrl+u': 'Ctrl+U', 'shift+tab': 'Shift+Tab', bs: 'Backspace' };
const capLabel = { enter: 'Enter', esc: 'Esc', tab: 'Tab', space: 'Space', 'ctrl+p': 'Ctrl P', 'ctrl+d': 'Ctrl D',
  'ctrl+s': 'Ctrl S', 'ctrl+u': 'Ctrl U', 'shift+tab': 'Shift Tab' };

for (const s of take.steps) {
  if (s.hide) { out.push('Hide'); hidden = true; continue; }
  if (s.show) { out.push('Show'); hidden = false; continue; }
  if (s.mark) { events.push({ t, n, p, mark: s.mark }); continue; }
  if ('label' in s) { events.push({ t, n, p, label: s.label }); continue; }  // small status-bar label; '' clears
  if (s.sleep) { out.push(`Sleep ${Math.round(s.sleep * (hidden ? 1 : SLOW) * 1000)}ms`); vis(s.sleep); continue; }
  if (s.type) {  // literal text into an input; keycap shows the text
    const ms = s.speed ?? TYPE_MS;
    if (!hidden && s.cap !== false) events.push({ t, n, p: s.shell ? null : p, type: s.type, dur: (s.type.length * ms) / 1000 });
    if (!hidden) { n += s.type.length; if (!s.shell) p += s.type.length; }
    out.push(`Type@${Math.round(ms * (hidden ? 1 : SLOW))}ms ${JSON.stringify(s.type)}`);
    vis((s.type.length * ms) / 1000);
    continue;
  }
  if (s.keys) {  // single keys, each followed by a gap
    for (const k of s.keys) {
      if (!hidden && s.cap !== false) events.push({ t, n, p: s.shell ? null : p, key: capLabel[k] ?? k });
      if (!hidden) { n += 1; if (!s.shell) p += 1; }
      const kms = Math.round(TYPE_MS * (hidden ? 1 : SLOW));
      if (vhsKey[k]) out.push(/^(Ctrl|Shift)\+/.test(vhsKey[k]) ? vhsKey[k] : `${vhsKey[k]}@${kms}ms`);
      else out.push(`Type@${kms}ms ${JSON.stringify(k)}`);
      vis(TYPE_MS / 1000);
      const gap = s.gap ?? 0.35;
      out.push(`Sleep ${Math.round(gap * (hidden ? 1 : SLOW) * 1000)}ms`); vis(gap);
    }
    continue;
  }
  if (s.raw) { out.push(s.raw); continue; }
}
events.push({ t, n, p, mark: 'end' });

// overlay plan for cards.py / compose.py: labels run until the next label
const labels = {}, timeline = [];
const ls = events.filter((e) => 'label' in e);
ls.forEach((e, i) => {
  if (!e.label) return;
  const id = `l${i}`;
  labels[id] = e.label;
  const nx = ls[i + 1];
  timeline.push({ label: id, from: e.t + 0.15, nFrom: e.n, pFrom: e.p, to: (nx?.t ?? t) - 0.05, nTo: nx?.n ?? n, pTo: nx ? nx.p : null });
});
const keys = [...new Set(events.filter((e) => e.key).map((e) => e.key))];
fs.writeFileSync(`${W}/rec/${name}.cut.json`, JSON.stringify({ labels, timeline, keys }, null, 1));

fs.writeFileSync(`${W}/rec/${name}.tape`, out.join('\n') + '\n');
fs.writeFileSync(`${W}/rec/${name}.events.json`, JSON.stringify({ duration: t, slow: SLOW, events }, null, 1));
console.log(`${name}: ${t.toFixed(2)}s visible, ${events.length} events`);
