/**
 * Copy button for the one-line install command. Local script, no
 * dependencies; without JavaScript the command is still selectable text.
 */
(() => {
  for (const button of document.querySelectorAll('[data-copy]')) {
    button.addEventListener('click', async () => {
      const source = document.getElementById(button.dataset.copy);
      const status = document.getElementById(button.getAttribute('aria-describedby'));
      const text = source ? source.textContent.trim() : '';
      try {
        await navigator.clipboard.writeText(text);
        if (status) status.textContent = 'Copied. Paste it into a terminal.';
      } catch {
        // Clipboard blocked (e.g. insecure context): select it for a manual copy.
        const range = document.createRange();
        range.selectNodeContents(source);
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);
        if (status) status.textContent = 'Selected. Press Ctrl+C or Cmd+C to copy.';
      }
    });
  }
})();
