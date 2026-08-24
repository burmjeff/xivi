function copyWithSelection(text: string): boolean {
	if (typeof document === 'undefined' || !document.body) return false;

	const activeElement =
		document.activeElement instanceof HTMLElement ? document.activeElement : null;
	const selection = document.getSelection();
	const previousRange = selection?.rangeCount ? selection.getRangeAt(0).cloneRange() : null;
	const textarea = document.createElement('textarea');

	textarea.value = text;
	textarea.readOnly = true;
	textarea.setAttribute('aria-hidden', 'true');
	textarea.style.position = 'fixed';
	textarea.style.inset = '0 auto auto -9999px';
	textarea.style.opacity = '0';
	document.body.append(textarea);
	textarea.select();
	textarea.setSelectionRange(0, text.length);

	let copied = false;
	try {
		copied = document.execCommand('copy');
	} finally {
		textarea.remove();
		if (selection && previousRange) {
			selection.removeAllRanges();
			selection.addRange(previousRange);
		}
		activeElement?.focus({ preventScroll: true });
	}
	return copied;
}

/**
 * Copies text in HTTPS/localhost contexts using the Clipboard API and falls
 * back to the selection command supported by browsers on plain HTTP LAN URLs.
 */
export async function copyText(text: string): Promise<void> {
	if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return;
		} catch {
			// Permissions and browser policy can still reject a present API.
		}
	}

	if (!copyWithSelection(text)) {
		throw new Error('This browser did not allow clipboard access.');
	}
}
