import { useEffect, useState } from 'react';
import { api, MatchItem } from './api';
import { CardView } from './CardView';

export function Matches() {
  const [items, setItems] = useState<MatchItem[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void api.matches().then((m) => {
      setItems(m);
      setLoading(false);
    });
  }, []);

  if (loading) return <p>Loading…</p>;
  if (items.length === 0) return <p className="hint">No matches yet. Rate cards in the deck or search.</p>;
  return (
    <div>
      {items.map((m) =>
        m.card ? (
          <CardView key={m.match_id} card={m.card} />
        ) : (
          <div key={m.match_id} className="card">
            <p className="hint">Contact unavailable — the member deleted their profile.</p>
          </div>
        ),
      )}
    </div>
  );
}
