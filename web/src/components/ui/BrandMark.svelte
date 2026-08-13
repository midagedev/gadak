<script lang="ts">
  /*
   * 가, drawn as strands — not a typeset syllable.
   * ㄱ is the thread you follow; ㅏ is the other one.
   */
  let {
    size = 18,
    class: className = '',
    ...rest
  }: { size?: number; class?: string; [key: string]: unknown } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  aria-hidden="true"
  class={className}
  {...rest}
>
  <g transform="translate(-0.6 0.2)">
    <path
      d="M4.6 6.8c4.4-.6 8.6-.2 10.8 1.2c.25 3.6.2 7.4.1 11.4"
      stroke="currentColor"
      stroke-width="2.35"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <path
      d="M18.7 5.8c.04 4.6.04 9 0 13.6"
      stroke="currentColor"
      stroke-opacity="0.42"
      stroke-width="1.9"
      stroke-linecap="round"
    />
    <path
      d="M18.7 12.2h3.7"
      stroke="currentColor"
      stroke-opacity="0.42"
      stroke-width="1.9"
      stroke-linecap="round"
    />
  </g>
</svg>
