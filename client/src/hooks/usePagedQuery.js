import { useCallback, useEffect, useRef, useState } from 'react';
import apiClient from '../api/client';

// Server-side paginated list fetcher for the new {items,total,...} envelope.
// Debounces search (300ms) and resets to page 1 when the term changes.
export default function usePagedQuery(path, extraParams = {}) {
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPageRaw] = useState(1);
  const [perPage, setPerPageRaw] = useState(25);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadToken, setReloadToken] = useState(0);

  // Serialize extra params so the effect only reruns on real changes.
  const extraKey = JSON.stringify(extraParams);

  // Extra params change outside our setters; flag loading for the refetch
  // using the render-time previous-value comparison pattern from the React
  // docs, which avoids an extra effect pass.
  const [prevExtraKey, setPrevExtraKey] = useState(extraKey);
  if (extraKey !== prevExtraKey) {
    setPrevExtraKey(extraKey);
    setLoading(true);
  }

  // Flip loading in the user-invoked setters so the fetch effect never has to
  // call setState synchronously (which triggers cascading renders).
  const setPage = useCallback((value) => {
    setLoading(true);
    setPageRaw(value);
  }, []);

  const setPerPage = useCallback((value) => {
    setLoading(true);
    setPerPageRaw(value);
    setPageRaw(1);
  }, []);

  const refresh = useCallback(() => {
    setLoading(true);
    setReloadToken((prev) => prev + 1);
  }, []);

  // Debounce the search term; reset to the first page on a new term. The ref
  // mirrors debouncedSearch so a no-op run (mount, or typing then restoring
  // the same value) bails before touching loading and leaving it stuck.
  const debouncedSearchRef = useRef('');
  useEffect(() => {
    if (search === debouncedSearchRef.current) return undefined;
    const timer = setTimeout(() => {
      debouncedSearchRef.current = search;
      setLoading(true);
      setDebouncedSearch(search);
      setPageRaw(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  // Fetch whenever paging, the debounced term, or extra params change.
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    apiClient
      .get(path, {
        params: {
          page,
          per_page: perPage,
          search: debouncedSearch || undefined,
          ...JSON.parse(extraKey),
        },
        signal: controller.signal,
      })
      .then((response) => {
        if (!active) return;
        const data = response.data || {};
        setItems(Array.isArray(data.items) ? data.items : []);
        setTotal(typeof data.total === 'number' ? data.total : 0);
        setError('');
        setLoading(false);
      })
      .catch((err) => {
        if (!active || err.code === 'ERR_CANCELED') return;
        setError(err.response?.data?.error || 'Failed to load data');
        setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [path, page, perPage, debouncedSearch, extraKey, reloadToken]);

  return {
    items,
    total,
    page,
    perPage,
    search,
    loading,
    error,
    setPage,
    setPerPage,
    setSearch,
    refresh,
  };
}
