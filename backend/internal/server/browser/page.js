// Runs inside the page. Called as (function(){ <this file>; return __xc(op, arg) })().
// Elements get a data-xc-ref attribute so later calls can find them again.
function __xc(op, arg) {
  const top = window;
  const SKIP = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'SVG', 'HEAD', 'META', 'LINK']);
  const INTERACTIVE = new Set(['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'SUMMARY', 'LABEL']);
  const norm = (s) => (s || '').replace(/\s+/g, ' ').trim();
  const cut = (s, n) => (s.length > n ? s.slice(0, n) + '…' : s);

  function docs() {
    const out = [{ doc: document, frame: [] }];
    for (let i = 0; i < out.length && out.length < 20; i++) {
      const frames = out[i].doc.querySelectorAll('iframe,frame');
      frames.forEach((f, j) => {
        try {
          if (f.contentDocument && f.contentDocument.body) out.push({ doc: f.contentDocument, frame: out[i].frame.concat(j), el: f });
        } catch (e) {
          /* cross-origin frame */
        }
      });
    }
    return out;
  }

  function visible(el) {
    const view = el.ownerDocument.defaultView;
    const st = view.getComputedStyle(el);
    if (st.display === 'none' || st.visibility === 'hidden' || st.opacity === '0') return false;
    if (el.getAttribute('aria-hidden') === 'true') return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  }

  function nextRef() {
    top.__xcNext = (top.__xcNext || 0) + 1;
    return String(top.__xcNext);
  }

  function refOf(el) {
    let r = el.getAttribute('data-xc-ref');
    if (!r) {
      r = nextRef();
      el.setAttribute('data-xc-ref', r);
    }
    return r;
  }

  function byRef(ref) {
    for (const d of docs()) {
      const el = d.doc.querySelector('[data-xc-ref="' + String(ref).replace(/"/g, '') + '"]');
      if (el) return el;
    }
    return null;
  }

  function isInteractive(el) {
    if (INTERACTIVE.has(el.tagName)) return !(el.tagName === 'INPUT' && el.type === 'hidden');
    const role = el.getAttribute('role');
    if (role && /^(button|link|tab|menuitem|checkbox|radio|option|combobox|textbox)$/.test(role)) return true;
    if (el.hasAttribute('onclick') || el.isContentEditable) return true;
    const view = el.ownerDocument.defaultView;
    return view.getComputedStyle(el).cursor === 'pointer' && el.children.length <= 3 && norm(el.innerText).length > 0 && norm(el.innerText).length < 40;
  }

  function ownText(el) {
    let s = '';
    el.childNodes.forEach((n) => {
      if (n.nodeType === 3) s += ' ' + n.textContent;
    });
    return norm(s);
  }

  function label(el) {
    const parts = [];
    const aria = el.getAttribute('aria-label');
    if (aria) parts.push(aria);
    if (el.id) {
      const l = el.ownerDocument.querySelector('label[for="' + CSS.escape(el.id) + '"]');
      if (l) parts.push(norm(l.innerText));
    }
    if (el.placeholder) parts.push(el.placeholder);
    if (el.title) parts.push(el.title);
    return norm(parts.join(' '));
  }

  const KIND = { A: '链接', BUTTON: '按钮', SELECT: '下拉框', TEXTAREA: '输入框', SUMMARY: '按钮', LABEL: '标签' };

  function describeLine(el) {
    const tag = el.tagName;
    const ref = refOf(el);
    if (tag === 'INPUT') {
      const t = (el.type || 'text').toLowerCase();
      if (t === 'submit' || t === 'button' || t === 'reset') return '[' + ref + '] 按钮 "' + cut(norm(el.value), 60) + '"';
      if (t === 'checkbox' || t === 'radio') return '[' + ref + '] ' + (t === 'checkbox' ? '勾选框' : '单选') + ' "' + cut(label(el) || el.name || '', 60) + '"' + (el.checked ? ' 已选' : '');
      const hidden = t === 'password' || el.hasAttribute('data-xc-secret');
      const val = hidden ? (el.value ? '已填' : '空') : cut(el.value || '', 60);
      return '[' + ref + '] 输入框 type=' + t + (el.name ? ' name=' + el.name : '') + (label(el) ? ' "' + cut(label(el), 60) + '"' : '') + ' 值=' + JSON.stringify(val);
    }
    if (tag === 'SELECT') {
      const opts = Array.from(el.options).slice(0, 15).map((o) => norm(o.text));
      const cur = el.selectedIndex >= 0 ? norm(el.options[el.selectedIndex].text) : '';
      return '[' + ref + '] 下拉框' + (el.name ? ' name=' + el.name : '') + ' 当前="' + cut(cur, 40) + '" 选项: ' + cut(opts.join(' | '), 200);
    }
    if (tag === 'TEXTAREA') return '[' + ref + '] 输入框' + (el.name ? ' name=' + el.name : '') + (label(el) ? ' "' + cut(label(el), 60) + '"' : '') + ' 值=' + JSON.stringify(cut(el.value || '', 60));
    const text = cut(norm(el.innerText) || label(el), 80);
    let line = '[' + ref + '] ' + (KIND[tag] || '可点击') + ' "' + text + '"';
    if (tag === 'A' && el.getAttribute('href') && !el.getAttribute('href').startsWith('javascript')) line += ' -> ' + cut(el.getAttribute('href'), 80);
    return line;
  }

  function snapshot(maxChars) {
    const lines = [];
    let size = 0;
    let truncated = false;
    function push(s) {
      if (size + s.length > maxChars) {
        truncated = true;
        return false;
      }
      lines.push(s);
      size += s.length + 1;
      return true;
    }
    function walk(el, depth) {
      if (truncated || SKIP.has(el.tagName)) return;
      if (el.tagName === 'IFRAME' || el.tagName === 'FRAME') {
        let inner = null;
        try {
          inner = el.contentDocument && el.contentDocument.body;
        } catch (e) {
          inner = null;
        }
        if (inner) {
          push('(框架开始)');
          walk(inner, depth);
          push('(框架结束)');
        } else if (visible(el)) {
          push('[' + refOf(el) + '] 外部框架 ' + cut(el.src || '', 100));
        }
        return;
      }
      if (!visible(el)) return;
      if (el.tagName === 'IMG' || el.tagName === 'CANVAS') {
        const src = el.getAttribute('src') || '';
        const hint = (src + ' ' + (el.id || '') + ' ' + (el.className || '') + ' ' + (el.alt || '')).toLowerCase();
        if (/captcha|verify|yzm|checkcode|validatecode|vcode|验证码/.test(hint)) push('[' + refOf(el) + '] 验证码图片');
        else if (el.alt) push('[' + refOf(el) + '] 图片 "' + cut(norm(el.alt), 40) + '"');
        return;
      }
      if (isInteractive(el)) {
        if (!push(describeLine(el))) return;
        if (el.tagName !== 'LABEL' && el.tagName !== 'A' && el.tagName !== 'BUTTON') return;
        if (el.querySelector('input,select,textarea,button')) {
          for (const c of el.children) walk(c, depth + 1);
        }
        return;
      }
      const own = ownText(el);
      if (own) {
        if (!push('[' + refOf(el) + '] ' + cut(own, 200))) return;
      }
      for (const c of el.children) walk(c, depth + 1);
    }
    if (document.body) walk(document.body, 0);
    return { url: location.href, title: document.title, text: lines.join('\n'), truncated: truncated };
  }

  function cssPath(el) {
    const parts = [];
    let cur = el;
    while (cur && cur.nodeType === 1 && cur.tagName !== 'HTML') {
      if (cur.id && /^[A-Za-z][\w-]*$/.test(cur.id)) {
        parts.unshift('#' + cur.id);
        break;
      }
      let i = 1;
      let sib = cur;
      while ((sib = sib.previousElementSibling)) if (sib.tagName === cur.tagName) i++;
      parts.unshift(cur.tagName.toLowerCase() + ':nth-of-type(' + i + ')');
      cur = cur.parentElement;
    }
    return parts.join(' > ');
  }

  function anchorOf(el) {
    // A label next to the element, used to find a value whose text changes.
    let cur = el;
    for (let i = 0; i < 3 && cur; i++) {
      let sib = cur.previousElementSibling;
      while (sib) {
        const t = norm(sib.innerText);
        if (t) return cut(t, 40);
        sib = sib.previousElementSibling;
      }
      cur = cur.parentElement;
    }
    return '';
  }

  function frameOf(el) {
    for (const d of docs()) if (d.doc === el.ownerDocument) return d.frame;
    return [];
  }

  function describe(ref) {
    const el = byRef(ref);
    if (!el) return null;
    return {
      tag: el.tagName.toLowerCase(),
      id: el.id || '',
      name: el.getAttribute('name') || '',
      type: el.getAttribute('type') || '',
      placeholder: el.getAttribute('placeholder') || '',
      aria: el.getAttribute('aria-label') || '',
      text: cut(norm(el.innerText || el.value || ''), 80),
      css: cssPath(el),
      anchor: anchorOf(el),
      frame: frameOf(el),
    };
  }

  function frameDoc(path) {
    let doc = document;
    for (const i of path || []) {
      const f = doc.querySelectorAll('iframe,frame')[i];
      try {
        doc = f && f.contentDocument;
      } catch (e) {
        doc = null;
      }
      if (!doc) return null;
    }
    return doc;
  }

  function locate(loc) {
    const doc = frameDoc(loc.frame);
    if (!doc) return null;
    const tag = (loc.tag || '*').toLowerCase();
    const pick = (list) => {
      const vis = list.filter(visible);
      return vis.length ? vis[0] : null;
    };
    const tries = [];
    if (loc.id) tries.push(() => {
      const el = doc.getElementById(loc.id);
      return el && el.tagName.toLowerCase() === tag ? el : null;
    });
    if (loc.region) {
      if (loc.css) tries.push(() => pick(Array.from(doc.querySelectorAll(loc.css))));
      if (loc.anchor) tries.push(() => {
        const all = Array.from(doc.querySelectorAll(tag));
        return pick(all.filter((el) => anchorOf(el) === loc.anchor));
      });
    } else {
      if (loc.name) tries.push(() => pick(Array.from(doc.querySelectorAll(tag + '[name="' + CSS.escape(loc.name) + '"]'))));
      if (loc.text) tries.push(() => pick(Array.from(doc.querySelectorAll(tag)).filter((el) => cut(norm(el.innerText || el.value || ''), 80) === loc.text)));
      if (loc.placeholder) tries.push(() => pick(Array.from(doc.querySelectorAll(tag + '[placeholder="' + CSS.escape(loc.placeholder) + '"]'))));
      if (loc.aria) tries.push(() => pick(Array.from(doc.querySelectorAll(tag + '[aria-label="' + CSS.escape(loc.aria) + '"]'))));
      // By position only when the text still matches, so a moved layout
      // never turns a click on one button into a click on another.
      if (loc.css) tries.push(() => pick(Array.from(doc.querySelectorAll(loc.css)).filter((el) => !loc.text || cut(norm(el.innerText || el.value || ''), 80) === loc.text)));
    }
    for (const t of tries) {
      let el = null;
      try {
        el = t();
      } catch (e) {
        el = null;
      }
      if (el) return refOf(el);
    }
    return null;
  }

  function point(ref) {
    const el = byRef(ref);
    if (!el) return null;
    el.scrollIntoView({ block: 'center', inline: 'center' });
    const r = el.getBoundingClientRect();
    let x = r.left + r.width / 2;
    let y = r.top + r.height / 2;
    let win = el.ownerDocument.defaultView;
    while (win && win !== top && win.frameElement) {
      const fr = win.frameElement.getBoundingClientRect();
      x += fr.left;
      y += fr.top;
      win = win.parent;
    }
    return { x: x, y: y, w: r.width, h: r.height, visible: visible(el) };
  }

  function clear(ref) {
    const el = byRef(ref);
    if (!el) return false;
    el.focus();
    if ('value' in el) {
      el.value = '';
      el.dispatchEvent(new Event('input', { bubbles: true }));
    } else if (el.isContentEditable) {
      el.textContent = '';
    }
    return true;
  }

  function select(ref, value) {
    const el = byRef(ref);
    if (!el || el.tagName !== 'SELECT') return false;
    const opt = Array.from(el.options).find((o) => norm(o.text) === value || o.value === value);
    if (!opt) return false;
    el.value = opt.value;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    return true;
  }

  function info(ref) {
    const el = byRef(ref);
    if (!el) return null;
    return { tag: el.tagName.toLowerCase(), type: (el.getAttribute('type') || '').toLowerCase(), text: cut(norm(el.innerText || el.value || label(el)), 120) };
  }

  function check() {
    let password = false;
    let captcha = '';
    for (const d of docs()) {
      d.doc.querySelectorAll('input[type=password]').forEach((el) => {
        if (visible(el)) password = true;
      });
      if (!captcha) {
        d.doc.querySelectorAll('img,canvas,input').forEach((el) => {
          if (captcha || !visible(el)) return;
          const hint = ((el.getAttribute('src') || '') + ' ' + el.id + ' ' + el.className + ' ' + (el.getAttribute('alt') || '') + ' ' + (el.getAttribute('placeholder') || '') + ' ' + (el.getAttribute('name') || '')).toLowerCase();
          if (/captcha|yzm|checkcode|validatecode|vcode|验证码/.test(hint)) captcha = '图形验证码';
        });
      }
      if (!captcha) {
        d.doc.querySelectorAll('iframe').forEach((el) => {
          if (!captcha && visible(el) && /geetest|tcaptcha|captcha|turing\.captcha|recaptcha|hcaptcha/.test(el.src || '')) captcha = '滑块或点选验证';
        });
      }
      if (!captcha && d.doc.body && /拖动滑块|请完成安全验证|向右滑动|请按顺序点击/.test(d.doc.body.innerText || '')) captcha = '滑块或点选验证';
    }
    return { password: password, captcha: captcha, url: location.href };
  }

  // B214: the image of a character captcha and the field its answer goes in.
  const CAPTCHA = /captcha|yzm|checkcode|validatecode|vcode|verifycode|验证码/;
  const NOT_IMAGE_CODE = /短信|手机|sms|mobile|phone|邮箱|email/;
  function hintOf(el) {
    return ((el.getAttribute('src') || '') + ' ' + el.id + ' ' + el.className + ' ' + (el.getAttribute('alt') || '') + ' ' + (el.getAttribute('placeholder') || '') + ' ' + (el.getAttribute('name') || '') + ' ' + (el.getAttribute('aria-label') || '') + ' ' + label(el)).toLowerCase();
  }
  function textField(el) {
    if (el.tagName !== 'INPUT' || !visible(el) || el.disabled || el.readOnly) return false;
    const t = (el.getAttribute('type') || 'text').toLowerCase();
    return t === 'text' || t === 'tel' || t === 'number' || t === '';
  }
  function captcha() {
    if (check().captcha !== '图形验证码') return null;
    for (const d of docs()) {
      const img = Array.from(d.doc.querySelectorAll('img,canvas')).find((el) => visible(el) && CAPTCHA.test(hintOf(el)));
      if (!img) continue;
      const fields = Array.from(d.doc.querySelectorAll('input')).filter(textField);
      let input = fields.find((el) => CAPTCHA.test(hintOf(el)) && !NOT_IMAGE_CODE.test(hintOf(el)));
      // No hint on the field: the empty text field next to the image.
      for (let p = img.parentElement, i = 0; !input && p && i < 4; p = p.parentElement, i++) {
        input = fields.find((el) => p.contains(el) && !el.value);
      }
      if (!input) return null;
      return { image: refOf(img), input: refOf(input) };
    }
    return null;
  }

  function box(ref) {
    // Where the element is on the page, in CSS pixels, for a screenshot of it.
    const el = byRef(ref);
    if (!el) return null;
    el.scrollIntoView({ block: 'center', inline: 'center' });
    const r = el.getBoundingClientRect();
    let x = r.left;
    let y = r.top;
    let win = el.ownerDocument.defaultView;
    while (win && win !== top && win.frameElement) {
      const fr = win.frameElement.getBoundingClientRect();
      x += fr.left;
      y += fr.top;
      win = win.parent;
    }
    return { x: x + top.scrollX, y: y + top.scrollY, w: r.width, h: r.height };
  }

  function common(refs) {
    // The smallest element that holds all of refs, so a list can be read as
    // one area even when its container has no text of its own.
    const els = (refs || []).map(byRef);
    if (!els.length || els.some((e) => !e)) return null;
    let cur = els[0];
    while (cur && !els.every((e) => cur.contains(e))) cur = cur.parentElement;
    return cur ? refOf(cur) : null;
  }

  function relPath(from, el) {
    const parts = [];
    for (let cur = el; cur !== from; cur = cur.parentElement) {
      let i = 1;
      let sib = cur;
      while ((sib = sib.previousElementSibling)) if (sib.tagName === cur.tagName) i++;
      parts.unshift(cur.tagName.toLowerCase() + ':nth-of-type(' + i + ')');
    }
    return parts.length ? ':scope > ' + parts.join(' > ') : ':scope';
  }

  function shape(arg) {
    // fields are in the first record of a list. The smallest element that
    // holds them all is one record; other records sit at the same depth.
    const region = byRef(arg.region);
    const fields = (arg.fields || []).map(byRef);
    if (!region || !fields.length || fields.some((e) => !e || !region.contains(e))) return null;
    let rec = fields[0];
    while (rec && !fields.every((e) => rec.contains(e))) rec = rec.parentElement;
    if (!rec || !region.contains(rec)) return null;
    const row = [];
    for (let cur = rec; cur !== region; cur = cur.parentElement) row.unshift(cur.tagName.toLowerCase());
    return { row: row, class: Array.from(rec.classList), paths: fields.map((e) => relPath(rec, e)) };
  }

  function rows(arg) {
    const region = byRef(arg.region);
    if (!region) return null;
    let set = [region];
    for (const tag of arg.row || []) {
      const next = [];
      for (const el of set) for (const c of el.children) if (c.tagName.toLowerCase() === tag) next.push(c);
      set = next;
    }
    const out = [];
    // A header row often has the same tags. It must share a class with the
    // first record when that record has any.
    const cls = arg.class || [];
    for (const r of set) {
      if (!visible(r)) continue;
      if (cls.length && !cls.some((c) => r.classList.contains(c))) continue;
      const vals = [];
      let ok = true;
      for (const p of arg.paths || []) {
        let el = null;
        try {
          el = p === ':scope' ? r : r.querySelector(p);
        } catch (e) {
          el = null;
        }
        if (!el || el.hasAttribute('data-xc-secret')) {
          ok = false;
          break;
        }
        vals.push(norm(el.innerText || el.value || ''));
      }
      if (ok) out.push(vals);
    }
    return out;
  }

  function text(ref) {
    const el = byRef(ref);
    if (!el) return null;
    return norm(el.innerText || el.value || '');
  }

  function el_secret(ref) {
    const el = byRef(ref);
    return !!(el && el.hasAttribute('data-xc-secret'));
  }

  function storage() {
    const out = {};
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const k = localStorage.key(i);
        out[k] = localStorage.getItem(k);
      }
    } catch (e) {
      /* storage blocked */
    }
    return { origin: location.origin, items: out };
  }

  switch (op) {
    case 'snapshot':
      return snapshot(arg || 12000);
    case 'describe':
      return describe(arg);
    case 'locate':
      return locate(arg);
    case 'point':
      return point(arg);
    case 'clear':
      return clear(arg);
    case 'select':
      return select(arg.ref, arg.value);
    case 'info':
      return info(arg);
    case 'captcha':
      return captcha();
    case 'box':
      return box(arg);
    case 'check':
      return check();
    case 'text':
      return el_secret(arg) ? null : text(arg);
    case 'common':
      return common(arg);
    case 'shape':
      return shape(arg);
    case 'rows':
      return rows(arg);
    case 'secret': {
      const el = byRef(arg);
      if (el) el.setAttribute('data-xc-secret', '1');
      return !!el;
    }
    case 'storage':
      return storage();
  }
  return null;
}
