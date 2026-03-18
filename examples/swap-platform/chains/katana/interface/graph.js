(() => {
  const DESKTOP_MIN = 1024;
  const NS = "http://www.w3.org/2000/svg";

  let rafId = null;
  let particles = [];
  let overlay = null;
  let svg = null;
  let groups = null;
  let resizeTimeout = null;
  let lastPulseAt = 0;
  let pulseCycle = 0;

  // Keep this small and curated.
  // Replace/add assets here as you like.
  const PULSE_ICONS = [
    { href: "/public/ethereum-logo.png", label: "ETH" },
    { href: "/public/katana-logo.svg", label: "KAT" },
    { href: "/public/token-default.svg", label: "TOK" },
    { href: "/public/defistate_hex_logo.svg", label: "DFS" },
    { href: "/public/ethereum-logo.png", label: "WETH" },
    { href: "/public/token-default.svg", label: "USDC" },
    { href: "/public/katana-logo.svg", label: "UNI" },
    { href: "/public/token-default.svg", label: "SUSHI" },
  ];

  function q(id) {
    return document.getElementById(id);
  }

  function createSvgEl(tag, attrs = {}) {
    const el = document.createElementNS(NS, tag);
    for (const [key, value] of Object.entries(attrs)) {
      el.setAttribute(key, String(value));
    }
    return el;
  }

  function ensureOverlay(hero) {
    if (overlay) return;

    overlay = document.createElement("div");
    overlay.className = "state-graph-overlay";
    overlay.setAttribute("aria-hidden", "true");

    svg = createSvgEl("svg");

    groups = {
      defs: createSvgEl("defs"),
      rails: createSvgEl("g"),
      particleBack: createSvgEl("g"),
      particleFront: createSvgEl("g"),
    };

    svg.appendChild(groups.defs);
    svg.appendChild(groups.rails);
    svg.appendChild(groups.particleBack);
    svg.appendChild(groups.particleFront);
    overlay.appendChild(svg);
    hero.appendChild(overlay);
  }

  function clearGraph() {
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }

    particles = [];
    lastPulseAt = 0;

    if (!groups) return;
    groups.defs.innerHTML = "";
    groups.rails.innerHTML = "";
    groups.particleBack.innerHTML = "";
    groups.particleFront.innerHTML = "";
  }

  function pointOnRight(rect, heroRect, offsetY = 0) {
    return {
      x: rect.right - heroRect.left,
      y: rect.top - heroRect.top + rect.height / 2 + offsetY
    };
  }

  function pointOnLeft(rect, heroRect, offsetY = 0) {
    return {
      x: rect.left - heroRect.left,
      y: rect.top - heroRect.top + rect.height / 2 + offsetY
    };
  }

  function pointInsideSwap(rect, heroRect, xRatio, yRatio) {
    return {
      x: rect.left - heroRect.left + rect.width * xRatio,
      y: rect.top - heroRect.top + rect.height * yRatio
    };
  }

  function cubicPath(a, c1, c2, b) {
    return `M ${a.x} ${a.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${b.x} ${b.y}`;
  }

  function styleRail(path, emphasis = false, opacity = 0.82) {
    path.setAttribute("fill", "none");
    path.setAttribute("stroke", "rgba(15,23,42,0.88)");
    path.setAttribute("stroke-width", emphasis ? "2.4" : "1.9");
    path.setAttribute("stroke-linecap", "round");
    path.setAttribute("stroke-linejoin", "round");
    path.setAttribute("opacity", String(opacity));
  }

  function iconForRail(index) {
    return PULSE_ICONS[(pulseCycle + index) % PULSE_ICONS.length];
  }

  function createIconParticle(icon, particleIndex) {
    const clipId = `graph-pulse-clip-${pulseCycle}-${particleIndex}-${Math.random().toString(36).slice(2, 8)}`;
    const clipPath = createSvgEl("clipPath", { id: clipId });
    const clipCircle = createSvgEl("circle", { cx: "0", cy: "0", r: "6.4" });
    clipPath.appendChild(clipCircle);
    groups.defs.appendChild(clipPath);

    const back = createSvgEl("g");
    const outer = createSvgEl("circle", {
      cx: "0",
      cy: "0",
      r: "8.2"
    });
    outer.setAttribute("fill", "rgba(255,255,255,0.98)");
    outer.setAttribute("stroke", "rgba(15,23,42,0.10)");
    outer.setAttribute("stroke-width", "0.6");

    const innerRing = createSvgEl("circle", {
      cx: "0",
      cy: "0",
      r: "6.9"
    });
    innerRing.setAttribute("fill", "rgba(248,250,252,0.98)");

    back.appendChild(outer);
    back.appendChild(innerRing);

    const front = createSvgEl("g");

    const image = createSvgEl("image", {
      x: "-6.4",
      y: "-6.4",
      width: "12.8",
      height: "12.8",
      preserveAspectRatio: "xMidYMid slice",
      "clip-path": `url(#${clipId})`,
      href: icon.href
    });

    const fallbackCircle = createSvgEl("circle", {
      cx: "0",
      cy: "0",
      r: "6.4"
    });
    fallbackCircle.setAttribute("fill", "rgba(15,23,42,0.92)");

    const fallbackText = createSvgEl("text", {
      x: "0",
      y: "0.9",
      "text-anchor": "middle",
      "font-size": "3.1",
      "font-weight": "700"
    });
    fallbackText.setAttribute("fill", "white");
    fallbackText.textContent = icon.label.slice(0, 3).toUpperCase();

    // Show both; image on top. If image fails, fallback remains visible beneath.
    front.appendChild(fallbackCircle);
    front.appendChild(fallbackText);
    front.appendChild(image);

    groups.particleBack.appendChild(back);
    groups.particleFront.appendChild(front);

    return { back, front };
  }

  function addParticle(path, progress, icon, particleIndex) {
    const { back, front } = createIconParticle(icon, particleIndex);
    particles.push({ path, progress, back, front });
  }

  function buildGraph() {
    const hero = q("heroGrid");
    const setStateCard = q("setStateCard");
    const swapCard = q("swapCard");
    const sellPane = q("swapSellPane");
    const buyPane = q("swapBuyPane");
    const tokenIn = q("tokenInTrigger");
    const tokenOut = q("tokenOutTrigger");
    const swapButton = q("swapButton");
    const flipButton = q("flipButton");

    if (
      !hero || !setStateCard || !swapCard ||
      !sellPane || !buyPane || !tokenIn || !tokenOut || !swapButton || !flipButton
    ) {
      return;
    }

    if (window.innerWidth < DESKTOP_MIN) {
      if (overlay) overlay.style.display = "none";
      clearGraph();
      return;
    }

    ensureOverlay(hero);
    overlay.style.display = "block";
    clearGraph();

    const heroRect = hero.getBoundingClientRect();
    const setRect = setStateCard.getBoundingClientRect();
    const swapRect = swapCard.getBoundingClientRect();
    const sellRect = sellPane.getBoundingClientRect();
    const buyRect = buyPane.getBoundingClientRect();
    const tokenInRect = tokenIn.getBoundingClientRect();
    const tokenOutRect = tokenOut.getBoundingClientRect();
    const buttonRect = swapButton.getBoundingClientRect();
    const flipRect = flipButton.getBoundingClientRect();

    const width = Math.ceil(heroRect.width);
    const height = Math.ceil(heroRect.height);

    svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
    svg.setAttribute("width", width);
    svg.setAttribute("height", height);

    const source = pointOnRight(setRect, heroRect, 0);

    const targets = [
      pointInsideSwap(swapRect, heroRect, 0.04, 0.18),
      pointOnLeft(sellRect, heroRect, -58),
      pointOnLeft(tokenInRect, heroRect, 0),
      pointOnLeft(flipRect, heroRect, 0),
      pointOnLeft(buyRect, heroRect, 58),
      pointOnLeft(tokenOutRect, heroRect, 0),
      pointInsideSwap(swapRect, heroRect, 0.04, 0.86),
      pointOnLeft(buttonRect, heroRect, 0),
    ];

    const gapWidth = swapRect.left - setRect.right;
    const c1x = source.x + Math.max(70, gapWidth * 0.22);
    const c2x = source.x + Math.max(190, gapWidth * 0.72);

    const topY = targets[0].y;
    const bottomY = targets[targets.length - 1].y;
    const spread = bottomY - topY || 1;

    const railPaths = [];

    targets.forEach((target, index) => {
      const normalized = (target.y - topY) / spread;
      const startY = source.y + (normalized - 0.5) * 18;

      const start = {
        x: source.x,
        y: startY
      };

      const path = createSvgEl("path", {
        d: cubicPath(
          start,
          {
            x: c1x,
            y: startY
          },
          {
            x: c2x,
            y: target.y
          },
          target
        )
      });

      styleRail(path, index === 3, index === 3 ? 0.92 : 0.8);
      groups.rails.appendChild(path);
      railPaths.push(path);
    });

    railPaths.forEach((path, index) => {
      addParticle(path, 0, iconForRail(index), index);
    });

    let last = performance.now();
    lastPulseAt = performance.now();

    function animate(now) {
      const dt = Math.min((now - last) / 1000, 0.05);
      last = now;

      const PULSE_INTERVAL = 3000;

      if (now - lastPulseAt >= PULSE_INTERVAL) {
        lastPulseAt += PULSE_INTERVAL;
        pulseCycle += 1;

        particles.forEach((p, index) => {
          p.progress = 0;

          if (p.back?.parentNode) p.back.parentNode.removeChild(p.back);
          if (p.front?.parentNode) p.front.parentNode.removeChild(p.front);

          const { back, front } = createIconParticle(iconForRail(index), index);
          p.back = back;
          p.front = front;
        });
      }

      for (const p of particles) {
        p.progress += dt;
        if (p.progress > 1) p.progress = 1;

        const len = p.path.getTotalLength();
        const pt = p.path.getPointAtLength(p.progress * len);

        p.back.setAttribute("transform", `translate(${pt.x.toFixed(2)} ${pt.y.toFixed(2)})`);
        p.front.setAttribute("transform", `translate(${pt.x.toFixed(2)} ${pt.y.toFixed(2)})`);
      }

      rafId = requestAnimationFrame(animate);
    }

    rafId = requestAnimationFrame(animate);
  }

  function rebuild() {
    buildGraph();
  }

  function onResize() {
    clearTimeout(resizeTimeout);
    resizeTimeout = setTimeout(buildGraph, 80);
  }

  window.addEventListener("load", rebuild);
  window.addEventListener("resize", onResize);
  document.addEventListener("DOMContentLoaded", rebuild);
})();