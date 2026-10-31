import { Card } from './api';

export function CardView({ card }: { card: Card }) {
  return (
    <div className="card">
      {card.first_name && (
        <h3>
          {card.first_name} {card.last_name ?? ''}
        </h3>
      )}
      <div className="tags">{card.skills.map((s) => <span key={s} className="tag">{s}</span>)}</div>
      {card.projects.length > 0 && (
        <div>
          <label>Projects</label>
          <ul>
            {card.projects.map((p, i) => (
              <li key={i}>{p}</li>
            ))}
          </ul>
        </div>
      )}
      {card.professional_interests.length > 0 && (
        <div>
          <label>Professional interests</label>
          <div className="tags">{card.professional_interests.map((s) => <span key={s} className="tag">{s}</span>)}</div>
        </div>
      )}
      {card.personal_interests.length > 0 && (
        <div>
          <label>Personal interests</label>
          <div className="tags">{card.personal_interests.map((s) => <span key={s} className="tag">{s}</span>)}</div>
        </div>
      )}
      <p className="hint">
        {card.location ?? '—'} · {card.meeting_format}
        {card.my_evaluation ? ` · your rating: ${card.my_evaluation}` : ''}
      </p>
      {card.telegram_username && <p>Contact: @{card.telegram_username}</p>}
      {!card.telegram_username && card.telegram_user_id && (
        <p>
          Contact:{' '}
          <a href={`tg://user?id=${card.telegram_user_id}`}>open in Telegram</a>
        </p>
      )}
    </div>
  );
}
