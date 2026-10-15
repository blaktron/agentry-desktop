import { main } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { creditCount, tierLabel, topUpOffer } from "../../subscription";
import { nameOf } from "../../homeLines";
import { hostOf } from "../Settings";

// Home's Profile card: who is signed in, the tier and
// the credit balance, read live from the registry as Settings → Profile
// reads them (nothing is stored). It edits nothing: the card opens Settings
// → Profile, and Top up credits opens the site, where the tier offers packs.
// Signed out it is a Sign in card.
export function ProfileCard({ registry, signedIn, profile, error, onOpenProfile, onSignIn, onTopUp }: {
  registry: string;
  // null until the settings are read, so the card never shows Sign in to
  // someone who is signed in.
  signedIn: boolean | null;
  profile: main.ProfileView | null;
  error: string | null;
  onOpenProfile: () => void;
  onSignIn: () => void;
  onTopUp: () => void;
}) {
  const host = hostOf(registry);
  if (signedIn === false) {
    return (
      <CardFrame
        title="Sign in"
        onOpen={onSignIn}
        className="home-card"
        blurb={<>Sign in to <span className="mono">{host}</span> to see your profile and credits here, and to run QuickAgents In Agentry.</>}
      />
    );
  }
  const user = profile?.user;
  const sub = profile?.subscription;
  return (
    <CardFrame
      title="Profile"
      onOpen={onOpenProfile}
      className="home-card"
      status={error ? "Your profile could not be read." : profile ? null : "Reading your profile…"}
    >
      {user && (
        <dl className="home-card-facts">
          <dt>Name</dt>
          <dd>{nameOf(user)}</dd>
          <dt>Handle</dt>
          <dd className="mono">{user.handle}</dd>
          <dt>Registry</dt>
          <dd className="mono">{host}</dd>
          {sub && (
            <>
              <dt>Plan</dt>
              <dd>{tierLabel(sub.tier)}{sub.memberOf ? `, through ${sub.memberOf}` : ""}</dd>
              <dt>Credits</dt>
              <dd>{creditCount(sub.balance)}</dd>
            </>
          )}
        </dl>
      )}
      {sub && topUpOffer(sub) && (
        <button type="button" className="link build-way-part" onClick={onTopUp}>Top up credits</button>
      )}
    </CardFrame>
  );
}
