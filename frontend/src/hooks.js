import { useEffect, useRef } from 'react';

// Runs fn immediately and then every `ms` while mounted. Always calls the latest fn.
export function usePolling(fn, ms) {
  const saved = useRef(fn);
  saved.current = fn;
  useEffect(() => {
    saved.current();
    const id = setInterval(() => saved.current(), ms);
    return () => clearInterval(id);
  }, [ms]);
}
