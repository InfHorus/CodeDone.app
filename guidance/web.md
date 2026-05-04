# Web

Create web interfaces that feel designed, not generated. The goal is to produce production-grade frontend work with a strong visual point of view, excellent usability, and premium execution. Avoid generic AI aesthetics, random decoration, and safe-but-forgettable component layouts.

Use this guidance when building or improving websites, landing pages, dashboards, admin panels, product pages, app screens, design systems, or standalone UI components.

---

## Core Standard

A high-quality web result must be:

- **Purposeful**: every section, component, visual effect, and interaction supports the product goal.
- **Distinctive**: the interface has a recognizable aesthetic direction, not a recycled SaaS template.
- **Usable**: visual ambition never damages clarity, accessibility, or task completion.
- **Cohesive**: typography, color, spacing, radius, borders, shadows, icons, and motion feel like one system.
- **Production-ready**: responsive, accessible, performant, maintainable, and cleanly structured.

The interface should look like it belongs to a serious product, studio, or startup that invested in design.

---

## Agent Workflow

Before writing code, silently resolve these decisions:

1. **Product context**: What is the interface for? Who uses it? What action should the user take?
2. **Aesthetic direction**: Choose one clear direction and commit to it.
3. **Hierarchy**: Decide what the user must notice first, second, and third.
4. **System rules**: Define spacing, type scale, colors, radius, surfaces, borders, and motion tokens.
5. **Signature detail**: Add one memorable design move: unusual layout, strong typography, special border treatment, animated reveal, custom background, editorial composition, or distinctive component styling.
6. **Implementation path**: Build the simplest robust code that achieves the vision.

Do not start from generic centered cards, blue/purple gradients, uniform rounded rectangles, or default dashboard grids unless the user specifically asks for that style.

---

## Pick a Strong Aesthetic Direction

Choose a specific visual language. Do not blend too many styles.

Examples:

- **Deep-tech command center**: dark graphite, thin borders, monospace accents, dense but readable data, glowing status indicators, precise geometry.
- **Luxury editorial**: large serif headlines, restrained palette, generous whitespace, delicate rules, high-quality imagery, subtle motion.
- **Industrial software**: utilitarian layout, strong grid, amber/green signal colors, technical labels, panels, instrumentation feel.
- **Neo-brutalist product**: sharp contrast, thick borders, oversized typography, intentionally raw composition, minimal shadows.
- **Soft premium SaaS**: warm neutrals, calm surfaces, subtle depth, refined icons, strong readability, restrained accent color.
- **Retro-futuristic interface**: scanlines/noise, unusual type, luminous accents, panelized composition, orbital or grid motifs.
- **Minimal executive tool**: almost no decoration, perfect spacing, quiet typography, excellent information density, polished states.
- **Playful consumer app**: expressive colors, bouncy motion, rounded forms, friendly illustrations, clear affordances.

Aesthetic direction should match the product. A cyberpunk treatment can fit a security product; it may be wrong for a medical billing dashboard.

---

## Avoid Generic AI Design

Never default to:

- Purple-to-blue gradients on a white background without a specific reason.
- Identical rounded cards in a predictable 3-column grid.
- Generic font stacks as the whole identity.
- Stock phrases like “seamless experience”, “unlock your potential”, or “next-generation platform” unless the user asks for marketing filler.
- Random glassmorphism, neon blobs, or huge shadows with no relation to the product.
- Overusing the same fonts across all generations, especially common AI defaults.
- Layouts where every section is centered, evenly spaced, and visually interchangeable.

A premium interface should have constraint, taste, and intent.

---

## Typography

Typography carries most of the perceived quality. Treat it as the foundation, not decoration.

### Rules

- Use a real type hierarchy: display, heading, body, caption, label, code/metadata when relevant.
- Pair fonts intentionally. Example: expressive display face + calm body face, or technical grotesk + monospace metadata.
- Avoid using only default system fonts unless the aesthetic is intentionally utilitarian.
- Use `clamp()` for fluid display sizes.
- Tighten large headlines slightly with negative letter spacing.
- Increase body line-height for readability.
- Use uppercase labels sparingly with tracked letter spacing.
- Do not make all text low contrast. Premium does not mean unreadable gray text.

### CSS snippet

```css
:root {
  --font-display: "Instrument Serif", Georgia, serif;
  --font-body: "Satoshi", "Aptos", system-ui, sans-serif;
  --font-mono: "IBM Plex Mono", "SFMono-Regular", monospace;

  --text-hero: clamp(3.5rem, 9vw, 8.5rem);
  --text-h1: clamp(2.4rem, 5vw, 5.25rem);
  --text-h2: clamp(1.75rem, 3vw, 3rem);
  --text-body: clamp(1rem, 1.2vw, 1.125rem);
}

.hero-title {
  font-family: var(--font-display);
  font-size: var(--text-hero);
  line-height: 0.88;
  letter-spacing: -0.055em;
}

.eyebrow {
  font-family: var(--font-mono);
  font-size: 0.72rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}
```

---

## Color

Color should be treated like product strategy. Use fewer colors, with more confidence.

### Rules

- Define color tokens with CSS variables.
- Pick one dominant background family, one text family, one surface family, and one or two accents.
- Use accent color for decisions, state, emphasis, or interaction, not random decoration.
- Prefer nuanced neutrals over pure black/white when building premium interfaces.
- Build contrast into the palette from the start.
- Use semantic state colors: success, warning, danger, info.
- Do not distribute colors evenly. Premium palettes usually have a dominant mood and precise accents.

### Palette snippet

```css
:root {
  --bg: #0b0d0f;
  --bg-elevated: #11151a;
  --surface: rgba(255, 255, 255, 0.055);
  --surface-strong: rgba(255, 255, 255, 0.095);
  --border: rgba(255, 255, 255, 0.13);
  --text: #f4f1e8;
  --muted: rgba(244, 241, 232, 0.66);
  --faint: rgba(244, 241, 232, 0.38);
  --accent: #d7ff4f;
  --accent-2: #69d8ff;
  --danger: #ff5c7a;
  --warning: #ffcc66;
  --success: #74f0a7;
}
```

---

## Layout and Composition

A polished interface rarely feels like stacked rectangles. Use composition deliberately.

### Rules

- Establish a grid, then break it intentionally.
- Use asymmetry when it improves memorability.
- Align aggressively. Misalignment should be a deliberate design move, not an accident.
- Use generous whitespace for luxury/refined products; use controlled density for command-center or technical products.
- Give sections different silhouettes: split hero, side rail, overlapping cards, editorial columns, sticky panels, staggered grids.
- Avoid equal visual weight everywhere. One element should dominate.
- Make mobile layouts feel designed, not just collapsed.

### Grid-breaking hero snippet

```css
.hero {
  min-height: 100svh;
  display: grid;
  grid-template-columns: minmax(1rem, 1fr) minmax(0, 74rem) minmax(1rem, 1fr);
  align-items: center;
  overflow: hidden;
}

.hero-inner {
  grid-column: 2;
  display: grid;
  grid-template-columns: 1.15fr 0.85fr;
  gap: clamp(2rem, 6vw, 7rem);
}

.hero-card {
  transform: translateY(3rem) rotate(-2deg);
  border: 1px solid var(--border);
  background: linear-gradient(180deg, var(--surface-strong), var(--surface));
  border-radius: 2rem;
}

@media (max-width: 820px) {
  .hero-inner {
    grid-template-columns: 1fr;
  }

  .hero-card {
    transform: none;
  }
}
```

---

## Surface, Depth, and Atmosphere

Premium interfaces need tactile surface design. Flat backgrounds are acceptable only when the typography and layout are exceptional.

Use:

- Subtle gradients that create lighting, not decoration.
- Hairline borders with alpha transparency.
- Inner highlights for glass, metal, or panel effects.
- Grain/noise overlays to prevent sterile digital flatness.
- Layered shadows with restraint.
- Contextual background motifs: grids for technical products, paper grain for editorial, soft radial light for luxury SaaS.

### Atmospheric background snippet

```css
.page {
  position: relative;
  min-height: 100svh;
  color: var(--text);
  background:
    radial-gradient(circle at 20% 10%, rgba(215, 255, 79, 0.16), transparent 30rem),
    radial-gradient(circle at 85% 20%, rgba(105, 216, 255, 0.12), transparent 28rem),
    linear-gradient(180deg, #0b0d0f 0%, #070809 100%);
  overflow: hidden;
}

.page::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  opacity: 0.12;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140' viewBox='0 0 140 140'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.75' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='140' height='140' filter='url(%23n)' opacity='.45'/%3E%3C/svg%3E");
  mix-blend-mode: overlay;
}
```

---

## Motion

Motion should create clarity, rhythm, and delight. It should not feel like every element is randomly sliding around.

### Rules

- Use one strong page-load sequence rather than many unrelated animations.
- Prefer transform and opacity for performance.
- Use short durations for UI feedback, longer durations for atmospheric entrance.
- Stagger related items.
- Respect `prefers-reduced-motion`.
- Use easing curves that feel intentional, not default.
- Motion should reinforce hierarchy: primary content arrives first, supporting content follows.

### CSS reveal snippet

```css
[data-reveal] {
  opacity: 0;
  transform: translateY(18px);
  animation: reveal 720ms cubic-bezier(.16, 1, .3, 1) forwards;
  animation-delay: var(--delay, 0ms);
}

@keyframes reveal {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  [data-reveal] {
    animation: none;
    opacity: 1;
    transform: none;
  }
}
```

### React + Framer Motion pattern

```tsx
import { motion } from "framer-motion";

const fadeUp = {
  hidden: { opacity: 0, y: 18 },
  show: { opacity: 1, y: 0 }
};

export function Section({ children }: { children: React.ReactNode }) {
  return (
    <motion.section
      initial="hidden"
      whileInView="show"
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}
      variants={fadeUp}
    >
      {children}
    </motion.section>
  );
}
```

---

## Components

Components should have personality without sacrificing predictability.

### Buttons

- Primary button should be visually obvious.
- Hover states should feel tactile.
- Focus states must be visible.
- Use loading, disabled, and active states where relevant.

```css
.button-primary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.65rem;
  min-height: 3rem;
  padding: 0 1.15rem;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  color: #0b0d0f;
  background: var(--accent);
  font-weight: 700;
  box-shadow: 0 1rem 2.5rem rgba(215, 255, 79, 0.18);
  transition: transform 180ms ease, box-shadow 180ms ease, filter 180ms ease;
}

.button-primary:hover {
  transform: translateY(-2px);
  filter: saturate(1.08);
  box-shadow: 0 1.25rem 3rem rgba(215, 255, 79, 0.25);
}

.button-primary:focus-visible {
  outline: 3px solid rgba(215, 255, 79, 0.38);
  outline-offset: 3px;
}
```

### Cards

Cards should not all look identical. Vary emphasis through size, placement, content hierarchy, surface strength, or border treatment.

```css
.card {
  position: relative;
  border: 1px solid var(--border);
  border-radius: 1.5rem;
  background:
    linear-gradient(180deg, rgba(255,255,255,.085), rgba(255,255,255,.035));
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.08),
    0 1.5rem 4rem rgba(0,0,0,.26);
  overflow: hidden;
}

.card::after {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(135deg, rgba(255,255,255,.12), transparent 35%);
  opacity: 0.55;
}
```

### Forms

Forms are where premium design often fails. Make them clear, calm, and resilient.

- Labels must be visible or programmatically clear.
- Inputs need focus, error, disabled, and success states.
- Error messages should be specific.
- Use spacing to group fields logically.
- Do not rely only on color for validation.

```css
.field {
  display: grid;
  gap: 0.45rem;
}

.field label {
  color: var(--muted);
  font-size: 0.84rem;
  font-weight: 650;
}

.input {
  min-height: 3rem;
  border: 1px solid var(--border);
  border-radius: 0.9rem;
  background: rgba(255,255,255,.055);
  color: var(--text);
  padding: 0 0.9rem;
  transition: border-color 160ms ease, box-shadow 160ms ease, background 160ms ease;
}

.input:focus {
  outline: none;
  border-color: rgba(215, 255, 79, 0.7);
  box-shadow: 0 0 0 4px rgba(215, 255, 79, 0.12);
  background: rgba(255,255,255,.075);
}
```

---

## Information Hierarchy

Good web design is mostly controlled attention.

Use hierarchy through:

- Size
- Weight
- Color contrast
- Placement
- Whitespace
- Motion order
- Surface elevation
- Icon/detail density
- Reading direction

For every screen, identify:

1. **Primary action**: the thing the user should do.
2. **Primary proof**: the thing that makes the user believe it.
3. **Primary object**: the data, product, person, file, server, event, or entity the screen is about.

Do not give all elements equal emphasis.

---

## Premium Details Checklist

Before finalizing, verify:

- The first viewport has a clear visual idea.
- Spacing is consistent and intentional.
- Typography has real hierarchy.
- Buttons, cards, inputs, nav, and modals have polished states.
- Mobile layout is not an afterthought.
- There is at least one signature detail.
- Colors are tokenized.
- The UI works without relying on placeholder lorem ipsum.
- Animations respect reduced motion.
- Interactive elements have visible focus states.
- Contrast is readable.
- Empty, loading, and error states are handled when relevant.
- The code is maintainable and not overloaded with decorative hacks.

---

## Responsive Design

Design mobile and desktop as related compositions, not the same layout squeezed down.

### Rules

- Use fluid type and spacing with `clamp()`.
- Collapse grids deliberately.
- Reorder content only when it improves comprehension.
- Keep tap targets at least 44px where possible.
- Avoid tiny metadata text on mobile.
- Check long words, code strings, URLs, tables, and buttons.

```css
:root {
  --space-page: clamp(1rem, 4vw, 4rem);
  --space-section: clamp(4rem, 9vw, 9rem);
}

.section {
  padding-block: var(--space-section);
  padding-inline: var(--space-page);
}

.auto-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr));
  gap: clamp(1rem, 2vw, 1.5rem);
}
```

---

## Accessibility

Premium design includes accessibility. Do not treat it as a separate pass.

Requirements:

- Use semantic HTML first.
- Maintain readable contrast.
- Add visible focus states.
- Use `button` for actions and `a` for navigation.
- Respect `prefers-reduced-motion`.
- Label form fields.
- Make icon-only buttons accessible with `aria-label`.
- Do not disable zoom.
- Ensure keyboard navigation works.
- Avoid conveying state with color alone.

```html
<button class="icon-button" aria-label="Open command menu">
  <svg aria-hidden="true" viewBox="0 0 24 24">...</svg>
</button>
```

---

## Performance

Visual quality should not create sluggish UX.

Rules:

- Animate `transform` and `opacity`, not layout-heavy properties.
- Avoid excessive blur on large fixed elements.
- Compress and size images correctly.
- Lazy-load below-the-fold media.
- Use SVG/CSS for simple decorative effects.
- Avoid massive JS for effects that CSS can handle.
- Keep DOM depth reasonable.
- Prefer progressive enhancement.

---

## Code Quality

Implementation should be clean enough to ship.

- Use design tokens instead of repeated magic values.
- Keep class naming meaningful.
- Separate structure from decoration where possible.
- Avoid inline styles unless dynamic values are necessary.
- Make components reusable but do not over-abstract prematurely.
- Include realistic content structure.
- Keep dependencies minimal unless they clearly improve the result.

### Token foundation

```css
:root {
  --radius-xs: 0.45rem;
  --radius-sm: 0.75rem;
  --radius-md: 1rem;
  --radius-lg: 1.5rem;
  --radius-xl: 2rem;

  --shadow-soft: 0 1rem 3rem rgba(0, 0, 0, 0.18);
  --shadow-hard: 0 0.75rem 0 rgba(0, 0, 0, 0.22);

  --ease-out: cubic-bezier(.16, 1, .3, 1);
  --ease-snap: cubic-bezier(.2, .8, .2, 1);
}
```

---

## Landing Pages

A premium landing page should not just list features. It should build belief.

Recommended structure:

1. **Hero with strong claim**: concrete, differentiated, and visually memorable.
2. **Proof strip**: metrics, logos, quotes, demos, technical facts, or credibility markers.
3. **Problem framing**: show what current alternatives get wrong.
4. **Product mechanism**: explain how it works with a diagram, workflow, or interactive visual.
5. **Feature sections**: tie features to user outcomes.
6. **Use cases**: show specific scenarios.
7. **Objection handling**: security, pricing, migration, performance, compatibility.
8. **CTA**: repeat the primary action with stronger confidence.

Avoid vague claims. Replace “AI-powered productivity platform” with a concrete mechanism and result.

---

## Dashboards and Apps

A premium dashboard is clear under pressure.

Rules:

- Put critical status and next action above decorative analytics.
- Use density carefully: more data can feel premium if alignment, grouping, and contrast are excellent.
- Design empty states, loading states, error states, and permission states.
- Use tables only when comparison matters; use cards or timelines when sequence/status matters.
- Keep destructive actions visually distinct and confirmed.
- Make filters and search obvious.
- Use sticky context for long workflows when helpful.

### Status pill snippet

```css
.status {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border: 1px solid color-mix(in srgb, var(--status-color) 38%, transparent);
  border-radius: 999px;
  padding: 0.35rem 0.62rem;
  color: var(--text);
  background: color-mix(in srgb, var(--status-color) 14%, transparent);
  font-size: 0.78rem;
  font-weight: 700;
}

.status::before {
  content: "";
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: var(--status-color);
  box-shadow: 0 0 1rem var(--status-color);
}
```

---

## Dark Mode

Dark mode is not just black backgrounds.

Rules:

- Use layered dark neutrals, not pure black everywhere.
- Text should be slightly warm or cool, not always pure white.
- Borders need enough alpha to define surfaces.
- Avoid large saturated neon fills; use them as accents.
- Shadows are less visible in dark mode, so use borders, inner highlights, gradients, and elevation contrast.

---

## Light Mode

Light mode can still feel premium.

Rules:

- Use warm off-white or subtle tinted backgrounds.
- Avoid low-contrast gray text.
- Use soft borders instead of heavy shadows.
- Add editorial typography or strong layout to avoid looking like a default document.
- Accents should be precise and restrained.

```css
:root[data-theme="light"] {
  --bg: #f6f1e8;
  --bg-elevated: #fffaf1;
  --surface: rgba(21, 18, 14, 0.045);
  --surface-strong: rgba(21, 18, 14, 0.075);
  --border: rgba(21, 18, 14, 0.13);
  --text: #17130e;
  --muted: rgba(23, 19, 14, 0.64);
  --accent: #123cff;
}
```

---

## Iconography and Visual Assets

Icons should support the product language.

- Use one icon style: outline, filled, duotone, technical, playful, etc.
- Keep stroke widths consistent.
- Do not use icons as filler.
- Prefer diagrams when explaining systems.
- Use screenshots, mock UI, or abstract product visuals instead of generic illustrations when possible.

---

## Copy and Microcopy

Text is part of the interface.

Rules:

- Be specific.
- Use action verbs.
- Replace vague marketing with product truth.
- Make buttons describe the result: “Generate report”, “Deploy model”, “Review changes”.
- Error messages should explain what happened and how to fix it.
- Empty states should tell users what to do next.

Bad:

> Unlock seamless innovation with our powerful platform.

Better:

> Turn a rough product request into reviewed implementation tickets, then ship each change through supervised agents.

---

## Premium Micro-Interactions

Use small interactions to make the product feel alive:

- Button lift or magnetic hover.
- Card border light following cursor.
- Copy-to-clipboard confirmation.
- Smooth tab indicator movement.
- Command menu reveal.
- Subtle loading skeletons.
- Row hover actions in tables.
- Toasts that confirm exact outcomes.

Do not make interactions gimmicky. The best micro-interactions make the interface feel responsive and confident.

### Cursor-light card snippet

```css
.interactive-card {
  --mx: 50%;
  --my: 50%;
  position: relative;
  border: 1px solid var(--border);
  background:
    radial-gradient(circle at var(--mx) var(--my), rgba(255,255,255,.16), transparent 14rem),
    var(--surface);
}
```

```js
document.querySelectorAll(".interactive-card").forEach((card) => {
  card.addEventListener("pointermove", (event) => {
    const rect = card.getBoundingClientRect();
    card.style.setProperty("--mx", `${event.clientX - rect.left}px`);
    card.style.setProperty("--my", `${event.clientY - rect.top}px`);
  });
});
```

---

## Review Rubric

Score the result before delivering it:

### 1. Visual Direction

- Is there a clear aesthetic point of view?
- Could this be recognized among other generated UIs?
- Does it fit the product context?

### 2. Craft

- Are type, spacing, colors, radius, and borders consistent?
- Are the details refined?
- Does it avoid accidental clutter?

### 3. Usability

- Is the primary action clear?
- Is the hierarchy obvious?
- Does it work on mobile?

### 4. Production Quality

- Is the code clean?
- Are states handled?
- Is it accessible?
- Is it performant?

### 5. Memorability

- What is the one thing someone remembers after seeing it?

If the answer is “nothing”, improve the design before finalizing.

---

## Fast Design Recipes

### Premium deep-tech SaaS

- Background: dark graphite with subtle radial lighting.
- Type: sharp grotesk or technical sans + monospace metadata.
- Components: panels, status pills, thin borders, precise icons.
- Accent: acid green, cyan, amber, or red-orange.
- Signature: command-center hero, animated grid, live system indicators.

### Luxury AI product

- Background: warm off-white or near-black.
- Type: elegant serif display + neutral sans body.
- Components: editorial cards, fine rules, large imagery, restrained CTAs.
- Accent: champagne, deep blue, oxblood, or muted gold.
- Signature: oversized headline with calm, sparse composition.

### Developer tool

- Background: dark or light neutral.
- Type: readable sans + serious monospace.
- Components: code blocks, diffs, file trees, command palette, logs.
- Accent: one strong operational color.
- Signature: real workflow preview instead of abstract marketing.

### Consumer mobile app landing

- Background: bright but controlled.
- Type: friendly display + highly readable body.
- Components: phone mockups, feature bubbles, playful transitions.
- Accent: energetic color pair.
- Signature: product-in-motion demo or interactive visual.

---

## When Improving an Existing UI

Respect what already works. Improve in layers:

1. Fix hierarchy.
2. Normalize spacing.
3. Improve typography.
4. Tokenize color and surfaces.
5. Upgrade components and states.
6. Add one signature visual move.
7. Add motion only after layout and hierarchy are strong.

Do not redesign everything unless the user asks for a full redesign.

---

## Final Output Expectations

When generating web code:

- Provide complete, working code when possible.
- Use semantic HTML.
- Include responsive behavior.
- Include accessible states.
- Use CSS variables or design tokens.
- Avoid placeholder-only design.
- Make the first screen visually strong.
- Keep the implementation aligned with the chosen aesthetic direction.

The result should feel like a finished product surface, not a wireframe with gradients.
