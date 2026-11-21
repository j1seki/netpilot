import React, { useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';

export function ExecutablePath({ path }: { path?: string }) {
  const [result, setResult] = useState('');
  const text = useRef<HTMLSpanElement>(null);
  if (!path) return null;

  const copy = async (event: React.MouseEvent) => {
    event.stopPropagation();
    try {
      const copied = await navigator.clipboard?.writeText(path).then(() => true, () => false);
      if (!copied) {
        const response = await fetch('/api/clipboard/copy', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: path }),
        });
        if (!response.ok || !(await response.json()).copied) throw new Error('Clipboard unavailable');
      }
      setResult('Copied');
    } catch {
      if (!text.current) return;
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents(text.current);
      selection?.removeAllRanges();
      selection?.addRange(range);
      setResult('Path selected. Use your system copy shortcut.');
    }
  };

  return <div className="executable-path font-mono">
    <span ref={text}>{path}</span>
    <button type="button" onClick={copy} title={result || 'Copy executable path'} aria-label="Copy executable path">
      {result === 'Copied' ? <Check size={12} /> : <Copy size={12} />}
    </button>
    <span className="sr-only" role="status">{result}</span>
  </div>;
}
