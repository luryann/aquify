/* aquify site engine: scroll-scrubbed stages, word reveal, hero water. No dependencies. */
(() => {
  const rm = matchMedia('(prefers-reduced-motion: reduce)').matches
  const root = document.documentElement
  if (rm) root.classList.add('rm')
  const $$ = (s, c = document) => [...c.querySelectorAll(s)]
  const clamp = (v, a = 0, b = 1) => Math.min(b, Math.max(a, v))
  const vh = () => innerHeight

  /* nav */
  const nav = document.querySelector('.nav')
  const burger = document.querySelector('.burger')
  burger?.addEventListener('click', () => {
    const open = nav.classList.toggle('open')
    burger.setAttribute('aria-expanded', open)
  })
  $$('.links a').forEach(a => a.addEventListener('click', () => nav.classList.remove('open')))
  $$('.dd-btn').forEach(b => b.addEventListener('click', e => { e.stopPropagation(); const d = b.parentElement; const o = d.classList.toggle('open'); b.setAttribute('aria-expanded', o) }))
  document.addEventListener('click', () => $$('.dd.open').forEach(d => d.classList.remove('open')))
  document.addEventListener('keydown', e => { if (e.key === 'Escape') $$('.dd.open').forEach(d => d.classList.remove('open')) })

  /* stages */
  const stages = $$('.stage').map(el => {
    if (el.dataset.len) el.style.setProperty('--len', el.dataset.len)
    const rs = $$('[data-r]', el).map(n => { const [a, b] = n.dataset.r.split(',').map(Number); return { n, a, b } })
    const steps = $$('.step', el), layers = $$('.layers .win', el)
    return { el, rs, steps, layers, count: +el.dataset.steps || steps.length || layers.length, p: 0, target: 0, sh: el.hasAttribute('data-shorthand') }
  })
  if (rm) stages.forEach(s => { s.el.classList.add('flow'); s.rs.forEach(r => r.n.style.setProperty('--t', 1)); s.layers.forEach(l => l.classList.add('on')); s.steps.forEach(x => x.classList.add('on')) })

  /* word statements: pinned stage, per-word opacity follows scroll */
  const says = $$('.say').map(el => {
    const p = el.querySelector('p')
    const words = p.textContent.trim().split(/\s+/)
    p.setAttribute('aria-label', p.textContent.trim())
    p.textContent = ''
    words.forEach((w, i) => { const sp = document.createElement('span'); sp.className = 'w'; sp.setAttribute('aria-hidden', 'true'); sp.textContent = w; p.append(sp); if (i < words.length - 1) p.append(' ') })
    return { el, ws: $$('.w', p) }
  })

  /* shorthand -> structured workout (numbers match the real quick-entry parser) */
  const SHORT = [
    { t: 'Warmup:\n', sec: null },
    { t: '400 choice\n', sec: 0 },
    { t: '8x50 kick w/ fins @1:00\n', sec: 0 },
    { t: 'Main\n', sec: null },
    { t: '3 x {4x100 free @1:30 EN2, 100 easy} rest 1:00\n', sec: 1 },
    { t: 'Cooldown: 200 easy', sec: 2 },
  ]
  const SECS = [['Warmup', 800], ['Main set', 1500], ['Cooldown', 200]]
  const FULL = SHORT.map(s => s.t).join('')
  function renderShorthand(s, p) {
    const typedEl = s.el.querySelector('.typed'), out = s.el.querySelector('.out')
    const n = Math.floor(clamp(p / 0.78) * FULL.length)
    const done = new Set()
    let acc = 0
    SHORT.forEach(l => { acc += l.t.length; if (n >= acc - (l.t.endsWith('\n') ? 1 : 0) && l.sec !== null) done.add(l.sec) })
    typedEl.innerHTML = FULL.slice(0, n).replace(/</g, '&lt;') + '<span class="cur"></span>'
    $$('.sec', out).forEach((el, i) => el.classList.toggle('on', done.has(i)))
    const total = [...done].reduce((a, i) => a + SECS[i][1], 0)
    const shown = p > .86 ? 2500 : total
    out.querySelector('[data-total]').textContent = shown.toLocaleString('en-US')
    out.querySelector('[data-lengths]').textContent = (shown / 25).toLocaleString('en-US')
    out.querySelector('.zones').classList.toggle('on', p > .86)
  }

  /* counters */
  const counters = $$('[data-count]')

  function update(force) {
    const H = vh()
    nav?.classList.toggle('solid', scrollY > 24)
    const hero = document.querySelector('.hero')
    if (hero) hero.style.setProperty('--hp', clamp(scrollY / (H * .8)).toFixed(4))

    for (const s of stages) {
      const r = s.el.getBoundingClientRect()
      if (r.bottom < -H || r.top > H * 2) continue
      s.target = clamp(-r.top / (r.height - H))
      if (rm || force) s.p = rm ? 1 : s.target
      else s.p += (s.target - s.p) * .14
      if (Math.abs(s.target - s.p) < .0005) s.p = s.target
      const p = s.p
      s.el.style.setProperty('--p', p.toFixed(4))
      for (const x of s.rs) x.n.style.setProperty('--t', clamp((p - x.a) / (x.b - x.a)).toFixed(4))
      if (s.count && !rm) {
        const idx = Math.min(s.count - 1, Math.floor(p * s.count * .999))
        s.steps.forEach((e, i) => e.classList.toggle('on', i === idx))
        s.layers.forEach((e, i) => e.classList.toggle('on', i === idx))
      }
      if (s.sh) renderShorthand(s, rm ? 1 : p)
    }

    for (const w of says) {
      const st = stages.find(s => s.el === w.el)
      const q = rm ? 1 : clamp((st.p - .08) / .74)
      const k = q * w.ws.length
      w.ws.forEach((e, i) => { e.style.opacity = (.12 + .88 * clamp(k - i)).toFixed(3) })
    }

    $$('[data-par]').forEach(e => {
      if (rm) return
      const r = e.getBoundingClientRect()
      e.style.transform = `translate3d(0,${((r.top + r.height / 2 - H / 2) * -parseFloat(e.dataset.par)).toFixed(1)}px,0)`
    })

    counters.forEach(c => {
      if (c.dataset.done) return
      if (c.getBoundingClientRect().top < H * .85) {
        c.dataset.done = 1
        const end = parseFloat(c.dataset.count), dec = (c.dataset.count.split('.')[1] || '').length, t0 = performance.now()
        const tick = now => { const k = rm ? 1 : clamp((now - t0) / 1400), e = 1 - Math.pow(1 - k, 4); c.textContent = (end * e).toFixed(dec); if (k < 1) requestAnimationFrame(tick) }
        requestAnimationFrame(tick)
      }
    })
  }

  let running = false
  const loop = () => { update(); if (stages.some(s => Math.abs(s.target - s.p) > .0005)) requestAnimationFrame(loop); else running = false }
  const kick = () => { if (!running) { running = true; requestAnimationFrame(loop) } }
  addEventListener('scroll', kick, { passive: true })
  addEventListener('resize', () => { update(true); kick() })
  update(true)
  kick()

  /* hero water: a caustics shader. Falls back to a CSS gradient. */
  const cv = document.getElementById('water')
  if (cv) {
    const gl = cv.getContext('webgl', { antialias: false, alpha: false, powerPreference: 'low-power' })
    const fb = document.querySelector('.hero .fallback, .page-hero .fallback')
    if (!gl) { cv.remove() } else {
      fb?.remove()
      const vs = 'attribute vec2 p;void main(){gl_Position=vec4(p,0.,1.);}'
      const fs = `precision highp float;uniform vec2 R;uniform float T;uniform vec2 M;
      #define TAU 6.28318530718
      void main(){
        vec2 uv=gl_FragCoord.xy/R.xy; float asp=R.x/R.y;
        vec2 p=mod(vec2(uv.x*asp,uv.y)*TAU*1.15+(M-.5)*.8,TAU)-250.;
        vec2 i=p; float c=1.; float inten=.0055; float t=T*.45+23.;
        for(int n=0;n<5;n++){
          float tt=t*(1.-(3.5/float(n+1)));
          i=p+vec2(cos(tt-i.x)+sin(tt+i.y),sin(tt-i.y)+cos(tt+i.x));
          c+=1./length(vec2(p.x/(sin(i.x+tt)/inten),p.y/(cos(i.y+tt)/inten)));
        }
        c/=5.; c=1.17-pow(c,1.4);
        float w=pow(abs(c),8.);
        vec3 deep=vec3(.008,.03,.042), mid=vec3(.02,.17,.22), hi=vec3(.62,.93,.98);
        vec3 col=mix(deep,mid,clamp(w*1.4,0.,1.))+hi*clamp(w-.45,0.,1.)*.55;
        col*=.28+.72*smoothstep(1.4,.05,length(uv-vec2(.78,.82)));
        gl_FragColor=vec4(col,1.);
      }`
      const sh = (t, s) => { const o = gl.createShader(t); gl.shaderSource(o, s); gl.compileShader(o); return o }
      const pr = gl.createProgram(); gl.attachShader(pr, sh(gl.VERTEX_SHADER, vs)); gl.attachShader(pr, sh(gl.FRAGMENT_SHADER, fs)); gl.linkProgram(pr); gl.useProgram(pr)
      const b = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, b); gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW)
      const loc = gl.getAttribLocation(pr, 'p'); gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0)
      const uR = gl.getUniformLocation(pr, 'R'), uT = gl.getUniformLocation(pr, 'T'), uM = gl.getUniformLocation(pr, 'M')
      const scale = Math.min(devicePixelRatio, 1.25) * .6
      const size = () => { cv.width = Math.floor(cv.clientWidth * scale); cv.height = Math.floor(cv.clientHeight * scale); gl.viewport(0, 0, cv.width, cv.height) }
      size(); addEventListener('resize', size)
      let mx = .5, my = .5, vis = true
      addEventListener('pointermove', e => { mx = e.clientX / innerWidth; my = 1 - e.clientY / innerHeight }, { passive: true })
      new IntersectionObserver(([e]) => { vis = e.isIntersecting }).observe(cv)
      const draw = t => {
        if (vis) { gl.uniform2f(uR, cv.width, cv.height); gl.uniform1f(uT, rm ? 8 : t / 1000); gl.uniform2f(uM, mx, my); gl.drawArrays(gl.TRIANGLES, 0, 3) }
        if (!rm) requestAnimationFrame(draw)
      }
      requestAnimationFrame(draw)
    }
  }

  /* waitlist form */
  const form = document.querySelector('form.wl')
  if (form) form.addEventListener('submit', async e => {
    e.preventDefault()
    const msg = form.querySelector('.msg'), btn = form.querySelector('button')
    msg.className = 'msg'; msg.textContent = 'Sending…'; btn.disabled = true
    try {
      const r = await fetch(form.dataset.endpoint || '/api/waitlist', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(Object.fromEntries(new FormData(form))) })
      if (!r.ok) throw new Error(r.status)
      form.reset(); msg.className = 'msg ok'; msg.textContent = 'You’re on the list. We’ll email you when your club can switch.'
    } catch {
      msg.className = 'msg err'; msg.textContent = 'That didn’t send. Check your connection and try again.'
    } finally { btn.disabled = false }
  })
})()
