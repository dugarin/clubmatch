import { useCallback, useEffect, useState } from 'react';
import { api, Card } from './api';
import { CardView } from './CardView';

export function Deck() {
  const [cards, setCards] = useState<Card[]>([]);
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setCards(await api.deck());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const rate = async (value: string) => {
    const top = cards[0];
    if (!top) return;
    const res = await api.evaluate(top.id, value, 'deck');
    setCards(cards.slice(1));
    if (res.match) setNotice("It's a match! Check the Matches tab.");
  };

  if (loading) return <p>Loading…</p>;
  if (cards.length === 0)
    return (
      <p className="hint">
        The deck is empty for now. Cards you rated will return in 30 days; new members appear as they join.
      </p>
    );
  return (
    <div>
      {notice && <div className="banner">{notice}</div>}
      <CardView card={cards[0]} />
      <div className="actions">
        <button className="primary" style={{ background: '#999' }} onClick={() => void rate('not_interested')}>
          Not interested
        </button>
        <button className="primary" onClick={() => void rate('interested')}>
          Interested
        </button>
      </div>
    </div>
  );
}
