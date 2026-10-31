import { api, ProfileSelf } from './api';
import { Onboarding } from './Onboarding';

interface Props {
  profile: ProfileSelf;
  onSaved: (p: ProfileSelf) => void;
  onDeleted: () => void;
}

export function Profile({ profile, onSaved, onDeleted }: Props) {
  const remove = async () => {
    if (!window.confirm('Delete your profile? Your matches and ratings will be removed.')) return;
    await api.deleteProfile();
    onDeleted();
  };

  return (
    <div>
      {!profile.bot_linked && (
        <div className="banner">
          Notifications are off — start the bot to receive match and application updates.
        </div>
      )}
      {!profile.is_searchable && (
        <div className="banner">
          Your profile is hidden: a moderation removed your last skill. Add a skill to reappear.
        </div>
      )}
      <Onboarding initial={profile} onDone={onSaved} />
      <button className="danger" onClick={() => void remove()}>
        Delete my profile
      </button>
    </div>
  );
}
