<script module lang="ts">
  /** The two sizes a status dot comes in. `md` is the row lead (the one you
   *  aim at); `sm` is every dot that rides beside text it annotates. */
  export type DotSize = 'sm' | 'md'
  export const DOT_CLASS: Record<DotSize, string> = {
    sm: 'h-1.5 w-1.5',
    md: 'h-2 w-2',
  }
</script>

<script lang="ts">
  /*
   * A status-category dot (GDK-142 V10).
   *
   * The audit called the chips "a coin flip per file": the same three-bucket
   * ink was drawn at h-1 in one place, h-1.5 in the next and h-2 in a third,
   * and each caller reached into categoryMetaOf for the colour itself. One
   * owner now decides both, so a dot means the same thing wherever it lands.
   *
   * Anatomy rule (docs/project/UX_PRINCIPLES.md §16): a dot carries the
   * category and nothing else carries it again — the word beside it is the
   * site's own status name in muted ink, never re-tinted. Colour and word are
   * two facts, not one fact twice.
   *
   * IssueRow's lead dot is the documented exception: it is a filter button,
   * not a chip, so it owns its own hover affordance and pulls DOT_CLASS.md
   * from here for the size.
   */
  import { categoryMetaOf } from '../../lib/format'
  import type { StatusCategory } from '../../lib/types'

  let {
    cat,
    size = 'sm',
    title,
    class: klass = '',
  }: {
    cat: StatusCategory
    size?: DotSize
    /** The site's own status name — the dot's colour is the category, this is
     *  the exact status. */
    title?: string
    class?: string
  } = $props()
</script>

<span
  class="{DOT_CLASS[size]} flex-none rounded-full {klass}"
  style:background={categoryMetaOf(cat).color}
  {title}
></span>
