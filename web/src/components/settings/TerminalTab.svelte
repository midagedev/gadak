<script lang="ts">
  /*
   * The terminal's own settings (GDK-1357). Four groups:
   *  - appearance — dark (default) or follow the app theme. Written through
   *    on change like the footer theme picker: it is a look toggle, and Save
   *    reloads the page.
   *  - text — font size and family, the two `ui.tokens` leaves the pane
   *    reads (type.terminal, fonts.mono-terminal). Saved with the form.
   *  - behavior — scrollback and cursor blink. Saved with the form; new
   *    sessions pick them up.
   *  - shell and starting directory — read-only here, by design (GDK-1069):
   *    this document is reachable by paired devices, and those two name the
   *    binary the next terminal runs. The CLI on this machine sets them.
   */
  import { t } from '../../lib/i18n'
  import { copyText } from '../../lib/copy-text'
  import {
    TERMINAL_APPEARANCES,
    parseTerminalAppearance,
    persistTerminalAppearance,
    readTerminalAppearance,
    type TerminalAppearance,
  } from '../../lib/terminal/appearance'
  import { INPUT_BARE, SELECT_BARE, SELECT_CHEVRON, COPY_BTN } from './controls'
  import type { SettingsDraft } from './draft'
  import Icon from '../ui/Icon.svelte'

  let { draft = $bindable() }: { draft: SettingsDraft } = $props()

  let appearance = $state<TerminalAppearance>(readTerminalAppearance())

  function onAppearanceChange(event: Event): void {
    const next = parseTerminalAppearance((event.currentTarget as HTMLSelectElement).value)
    void persistTerminalAppearance(next)
    appearance = next
  }
  const SHELL_COMMANDS = 'gadak config set terminal.shell /bin/zsh\ngadak config set terminal.workingDir /path/to/work'
  let copiedShell = $state(false)
  async function copyShellCommands(): Promise<void> {
    if (await copyText(SHELL_COMMANDS)) {
      copiedShell = true
      setTimeout(() => {
        copiedShell = false
      }, 1500)
    }
  }
</script>

<div class="flex flex-col gap-5" data-testid="terminal-settings">
  <section class="flex flex-col gap-2">
    <label class="flex max-w-[280px] flex-col gap-1">
      <span class="text-micro font-medium tracking-wide text-text-muted uppercase">
        {t('settings.terminalAppearance')}
      </span>
      <span class="relative flex">
        <select
          class="{SELECT_BARE} w-full"
          data-testid="terminal-appearance-picker"
          value={appearance}
          onchange={onAppearanceChange}
        >
          {#each TERMINAL_APPEARANCES as mode (mode.name)}
            <option value={mode.name}>{t(mode.labelKey)}</option>
          {/each}
        </select>
        <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
      </span>
      <span class="text-micro leading-relaxed text-text-muted">
        {t('settings.terminalAppearanceDesc')}
      </span>
    </label>
  </section>

  <section class="flex flex-col gap-2 border-t border-border-subtle pt-4">
    <div class="text-micro font-medium tracking-wide text-text-muted uppercase">
      {t('settings.terminalText')}
    </div>
    <div class="flex flex-wrap gap-3">
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-secondary">{t('settings.terminalFontSize')}</span>
        <input
          class="{INPUT_BARE} w-24"
          type="number"
          min="9"
          max="24"
          placeholder="13"
          data-testid="terminal-font-size"
          bind:value={draft.terminalFontSizeText}
          oninput={() => (draft.uiTouched = true)}
        />
      </label>
      <label class="flex min-w-0 flex-1 flex-col gap-1">
        <span class="text-micro text-text-secondary">{t('settings.terminalFontFamily')}</span>
        <input
          class="{INPUT_BARE} w-full font-mono"
          type="text"
          spellcheck="false"
          placeholder="Menlo, Consolas, monospace"
          data-testid="terminal-font-family"
          bind:value={draft.terminalFontFamily}
          oninput={() => (draft.uiTouched = true)}
        />
      </label>
    </div>
    <p class="text-micro leading-relaxed text-text-muted">{t('settings.terminalTextDesc')}</p>
  </section>

  <section class="flex flex-col gap-2 border-t border-border-subtle pt-4">
    <div class="text-micro font-medium tracking-wide text-text-muted uppercase">
      {t('settings.terminalBehavior')}
    </div>
    <label class="flex max-w-[200px] flex-col gap-1">
      <span class="text-micro text-text-secondary">{t('settings.terminalScrollback')}</span>
      <input
        class="{INPUT_BARE} w-full"
        type="number"
        min="200"
        max="100000"
        placeholder="5000"
        data-testid="terminal-scrollback"
        bind:value={draft.terminalScrollbackText}
      />
      <span class="text-micro text-text-muted">{t('settings.terminalScrollbackDesc')}</span>
    </label>
    <label class="flex cursor-pointer items-start gap-2.5">
      <input
        type="checkbox"
        class="mt-0.5 flex-none accent-[var(--color-accent,#3b82f6)]"
        data-testid="terminal-cursor-blink"
        bind:checked={draft.terminalCursorBlink}
      />
      <span class="text-text-primary">{t('settings.terminalCursorBlink')}</span>
    </label>
    <p class="text-micro leading-relaxed text-text-muted">{t('settings.terminalBehaviorDesc')}</p>
  </section>

  <section class="flex flex-col gap-2 border-t border-border-subtle pt-4">
    <div class="text-micro font-medium tracking-wide text-text-muted uppercase">
      {t('settings.terminalShell')}
    </div>
    <p class="text-micro leading-relaxed text-text-muted">{t('settings.terminalShellDesc')}</p>
    <pre
      class="overflow-x-auto rounded-md border border-border-subtle bg-bg-panel px-3 py-2 font-mono text-micro text-text-secondary"
      data-testid="terminal-shell-commands">{SHELL_COMMANDS}</pre>
    <!-- Every other tab that shows a CLI command puts the copy button beside
         it (SyncTab, DevicesTab, IntegrationsTab); this one had none until
         the v0.20 audit read it. -->
    <div>
      <button type="button" class={COPY_BTN} onclick={() => void copyShellCommands()}>
        {copiedShell ? t('settings.copied') : t('settings.copy')}
      </button>
    </div>
  </section>
</div>
