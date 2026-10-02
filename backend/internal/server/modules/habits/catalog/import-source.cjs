// One-time content import. Run from the repository root with the supplied HTML path.
// Requires web/node_modules (jsdom) and Python with Pillow for WebP conversion.
// This is a development tool; the server never executes uploaded HTML or scripts.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { JSDOM } = require(path.join(process.cwd(), 'web/node_modules/jsdom'));
let source = fs.readFileSync(process.argv[2], 'utf8');
const digest = crypto.createHash('sha256').update(source).digest('hex');
const assetDir = path.join(process.cwd(), 'web/public/habits/personal');
fs.mkdirSync(assetDir, { recursive: true });
const images = new Map();
source = source.replace(/data:image\/[\w+.-]+;base64,[A-Za-z0-9+/=]+/g, (uri) => {
  const id = crypto.createHash('sha256').update(uri).digest('hex').slice(0, 16);
  if (!images.has(id)) images.set(id, uri.split(',')[1]);
  return `/habits/personal/${id}.webp`;
});
for (const [id, base64] of images) {
  if (fs.existsSync(path.join(assetDir, `${id}.webp`))) continue;
  const result = spawnSync('python', ['-c', 'import sys,io; from PIL import Image; im=Image.open(io.BytesIO(sys.stdin.buffer.read())).convert("RGB"); im.thumbnail((1800,1200)); im.save(sys.argv[1],"WEBP",quality=85,method=6)', path.join(assetDir, `${id}.webp`)], { input: Buffer.from(base64, 'base64') });
  if (result.status !== 0) throw new Error(result.stderr.toString());
}
const ctx = vm.createContext({ structuredClone, localStorage: { getItem: () => null }, window: { innerWidth: 1360 } });
for (const [, code] of source.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)) {
  const stop = code.indexOf("document.addEventListener('click'");
  vm.runInContext(stop < 0 ? code : code.slice(0, stop), ctx, { timeout: 2000 });
}
const run = (code) => JSON.parse(vm.runInContext(`JSON.stringify(${code})`, ctx, { timeout: 2000 }));
const raw = run('({exercises:EXERCISES,images:EXERCISE_IMAGES,sessions:SESSION,runLevels:RUN_LEVELS,habits:HABITS,phases:PHASES,original:ORIGINAL_CONTENT})');
const exercises = raw.exercises.map(e => ({ ...e, restored: !!e.restored, image: raw.images[e.id] }));
function md(node) {
  if (node.nodeType === 3) return node.textContent.replace(/\s+/g, ' ');
  if (node.nodeType !== 1) return '';
  const tag = node.tagName.toLowerCase();
  if (['input', 'select', 'textarea', 'script', 'style', 'source'].includes(tag)) return '';
  if (tag === 'label' || node.classList.contains('timer-panel') || node.classList.contains('page-heading')) return '';
  const body = () => [...node.childNodes].map(md).join('');
  if (/^h[1-6]$/.test(tag)) return `\n\n${'#'.repeat(Number(tag[1]))} ${body().trim()}\n\n`;
  if (tag === 'br') return '\n';
  if (tag === 'a') return `[${body().trim()}](${node.getAttribute('href')})`;
  if (tag === 'button') {
    const id = node.getAttribute('data-ex');
    return id ? `[${body().trim()}](/habits/fitness?exercise=${id})` : '';
  }
  if (tag === 'img') return `\n\n![${node.getAttribute('alt') || ''}](${node.getAttribute('src')})\n\n`;
  if (tag === 'b' || tag === 'strong') return `**${body().trim()}**`;
  if (tag === 'li') return `\n${node.parentElement.tagName === 'OL' ? [...node.parentElement.children].indexOf(node) + 1 + '.' : '-'} ${body().trim()}`;
  if (tag === 'table') {
    const rows = [...node.querySelectorAll('tr')].map(row => [...row.children].map(cell => md(cell).trim().replace(/\n/g, '<br>').replace(/\|/g, '\\|')));
    if (!rows.length) return '';
    return '\n\n' + rows.map((row, i) => `| ${row.join(' | ')} |\n${i === 0 ? '| ' + row.map(() => '---').join(' | ') + ' |\n' : ''}`).join('') + '\n';
  }
  if (['p', 'div', 'section', 'aside', 'ul', 'ol', 'picture'].includes(tag)) return `\n\n${body().trim()}\n\n`;
  return body();
}
const articles = [];
for (const [page, category] of [['habitPage', 'daily'], ['foodPage', 'food'], ['englishPage', 'english'], ['guidePage', 'reference'], ['originalPage', 'reference'], ['recordsPage', 'reference']]) {
  const doc = new JSDOM(run(`${page}()`)).window.document;
  for (const section of doc.querySelectorAll('.sheet,.food-intro,.notice')) {
    if (section.closest('.original-archive')) continue;
    const title = section.querySelector('h2')?.textContent.replace(/\s+/g, ' ').trim() || (category === 'food' ? '饮食说明' : '计划说明');
    if (page === 'recordsPage' && !['加重量的统一规则', '每四周，做一次复评'].includes(title)) continue;
    const text = md(section).replace(/\n{3,}/g, '\n\n').trim();
    if (text) articles.push({ id: `${category}-${articles.length + 1}`, title, category, content: text.replace(/^## [^\n]*\n+/, '') });
  }
}
const categories = { breakfast: 'food', lunch: 'food', protein: 'food', words: 'english', review: 'english' };
const habits = raw.habits.map(h => ({ ...h, category: categories[h.id] || 'daily', exerciseId: '' }));
for (const id of ['hang', 'thoracic', 'wallslide', 'chin']) {
  const e = exercises.find(e => e.id === id);
  habits.push({ id: `posture-${id}`, category: 'daily', group: '体态练习', name: e.name, short: e.name, tip: e.dose, exerciseId: id });
}
const sessions = Object.entries(raw.sessions).map(([id, s]) => ({ id, ...s, items: s.items.map(([exerciseId, sets, prescription, optional]) => ({ exerciseId, sets, prescription, optional: !!optional })) }));
const runLevels = raw.runLevels.map(([name, description, walkMinutes, runMinutes, rounds, durationMinutes], id) => ({ id, name, description, walkMinutes, runMinutes, rounds, durationMinutes }));
const phases = Object.entries(raw.phases).map(([id, [name, description]]) => ({ id: Number(id), name, description }));
const catalog = { version: 1, sourceHash: digest, exercises, sessions, runLevels, phases, habits, articles, original: raw.original };
fs.writeFileSync(path.join(__dirname, 'plan.json'), JSON.stringify(catalog, null, 2) + '\n');
console.log(JSON.stringify({ exercises: exercises.length, sessions: sessions.length, runLevels: runLevels.length, habits: habits.length, articles: articles.length, images: images.size }));
