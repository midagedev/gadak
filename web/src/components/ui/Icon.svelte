<script lang="ts" module>
  /*
   * The icon set. One component, one name→glyph map, one stroke weight.
   *
   * Why a wrapper rather than importing glyphs at each call site: an icon set is
   * only a set if every member shares a grid, a weight and a color contract.
   * Direct imports let each site pick its own size and inherit the library's
   * default 2px stroke, which is how a screen ends up with icons that visibly
   * belong to different families. Everything here renders on a 24-unit grid at
   * 1.5px absolute stroke in `currentColor`, so an icon takes the tone of the
   * text it sits beside — muted in the sidebar, red inside a reopen badge — and
   * needs no per-state color of its own.
   *
   * Glyphs come from Lucide, already a dependency and already rendering in the
   * detail and docs panels. Drawing a parallel set by hand would have produced
   * exactly the mismatch described above.
   *
   * A literal ✕ ✓ › ‹ ▾ in the markup is the same problem wearing a different
   * hat, which is why those are gone: a text glyph renders in whatever font the
   * platform picks for it, carries no stroke weight to match, and sits on the
   * text baseline instead of the icon grid, so a close button drawn with ✕ was
   * a different size and a different weight on macOS than beside it on Linux.
   * The exceptions are the ones that are genuinely punctuation and not a
   * control — · as a separator, — as a dash, × as the multiplication sign in
   * "Reopened 3×". Those stay text.
   */
  import {
    ArrowLeft,
    ArrowUpRight,
    Ban,
    Check,
    CheckCheck,
    ChevronLeft,
    ChevronRight,
    CircleCheck,
    CirclePlus,
    Clock,
    Eye,
    FileText,
    Film,
    FlaskConical,
    Globe,
    GripVertical,
    Hourglass,
    Inbox,
    Info,
    Layers,
    LayoutDashboard,
    Megaphone,
    MessageSquare,
    Paperclip,
    PenLine,
    Plus,
    RefreshCw,
    RotateCcw,
    Search,
    SearchX,
    Settings,
    Star,
    Terminal,
    TriangleAlert,
    UserRound,
    X,
    Zap,
  } from '@lucide/svelte'

  const ICONS = {
    'arrow-left': ArrowLeft,
    'arrow-up-right': ArrowUpRight,
    ban: Ban,
    check: Check,
    'check-check': CheckCheck,
    'check-circle': CircleCheck,
    'chevron-left': ChevronLeft,
    'chevron-right': ChevronRight,
    clock: Clock,
    eye: Eye,
    file: FileText,
    film: Film,
    flask: FlaskConical,
    globe: Globe,
    // Reorder handle (sidebar section headers). Not invented: Lucide's
    // GripVertical, same set as every other glyph in this map.
    grip: GripVertical,
    hourglass: Hourglass,
    inbox: Inbox,
    info: Info,
    layers: Layers,
    'layout-dashboard': LayoutDashboard,
    megaphone: Megaphone,
    'message-square': MessageSquare,
    paperclip: Paperclip,
    pen: PenLine,
    plus: Plus,
    'plus-circle': CirclePlus,
    // Fetching from the source. Distinct from rotate-ccw on purpose: that one
    // is spoken for as "reopened" (the built-in view and every reopen chip), so
    // reusing it for a sync would have meant one glyph and two meanings on the
    // same screen.
    refresh: RefreshCw,
    'rotate-ccw': RotateCcw,
    search: Search,
    'search-x': SearchX,
    settings: Settings,
    star: Star,
    terminal: Terminal,
    user: UserRound,
    warning: TriangleAlert,
    x: X,
    zap: Zap,
  } as const

  export type IconName = keyof typeof ICONS
</script>

<script lang="ts">
  let {
    name,
    size = 16,
    filled = false,
    class: cls = '',
    title,
  }: {
    name: IconName
    size?: number
    /** Solid interior — the one two-state glyph is the favorite star. */
    filled?: boolean
    class?: string
    title?: string
  } = $props()

  const Glyph = $derived(ICONS[name])
</script>

<!-- absoluteStrokeWidth: without it Lucide scales the stroke with the box, so
     the same icon reads heavier at 20px than at 14px. Pinning it keeps one
     weight across every size on screen. -->
<Glyph
  {size}
  strokeWidth={1.5}
  absoluteStrokeWidth
  fill={filled ? 'currentColor' : 'none'}
  class="flex-none {cls}"
  aria-hidden="true"
  {title}
/>
