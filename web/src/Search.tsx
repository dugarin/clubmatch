import { useState } from 'react';
import { api, ApiError, Card } from './api';
import { CardView } from './CardView';

export function Search() {
  const [q, setQ] = useState('');
  const [location, setLocation] = useState('');
  const [format, setFormat] = useState('');
  const [items, setItems] = useState<Card[]>([]);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);
  const [searched, setSearched] = useState(false);
  const [filtersApplied, setFiltersApplied] = useState(false);
  const [notice, setNotice] = useState('');

  const run = async (p: number) => {
    try {
      const res = await api.search(q, location, format, p);
      setItems(p === 1 ? res.items : [...items, ...res.items]);
      setPage(p);
      setHasMore(res.has_more);
      setSearched(true);
      setFiltersApplied(res.filters_applied);
      setNotice('');
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        setItems([]);
        setSearched(true);
        setNotice('Unknown tag. Search works by exact keywords (tags), e.g. "Kafka".');
      } else {
        throw e;
      }
    }
  };

  const rate = async (c: Card, value: string) => {
    const res = await api.evaluate(c.id, value, 'search');
    setItems(items.map((it) => (it.id === c.id ? { ...it, my_evaluation: value } : it)));
    if (res.match) setNotice("It's a match! Check the Matches tab.");
  };

  return (
    <div>
      {notice && <div className="banner">{notice}</div>}
      <input placeholder="Who are you looking for?" value={q} onChange={(e) => setQ(e.target.value)} />
      <div className="actions">
        <input placeholder="Location" value={location} onChange={(e) => setLocation(e.target.value)} />
        <select value={format} onChange={(e) => setFormat(e.target.value)}>
          <option value="">Any format</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
        </select>
      </div>
      <button className="primary" onClick={() => void run(1)} disabled={!q.trim()}>
        Search
      </button>
      {searched && items.length === 0 && (
        <p className="hint">
          No matches found.{filtersApplied && ' Try relaxing the filters.'}
        </p>
      )}
      {items.map((c) => (
        <div key={c.id}>
          <CardView card={c} />
          <div className="actions">
            <button className="primary" style={{ background: '#999' }} onClick={() => void rate(c, 'not_interested')}>
              Not interested
            </button>
            <button className="primary" onClick={() => void rate(c, 'interested')}>
              Interested
            </button>
          </div>
        </div>
      ))}
      {hasMore && (
        <button className="primary" onClick={() => void run(page + 1)}>
          More
        </button>
      )}
    </div>
  );
}
