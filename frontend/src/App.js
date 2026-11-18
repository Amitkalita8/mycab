import { useEffect, useState } from 'react';
import './App.css';
import { api, getToken, setToken } from './api';
import AuthScreen from './AuthScreen';
import DriverApp from './DriverApp';
import RiderApp from './RiderApp';

function App() {
  // undefined = checking saved login, null = logged out
  const [session, setSession] = useState(getToken() ? undefined : null);

  useEffect(() => {
    if (session !== undefined) return;
    api('/me')
      .then(setSession)
      .catch((err) => {
        // only drop the token if the server rejected it, not on a network blip
        if (err.status === 401) setToken(null);
        setSession(null);
      });
  }, [session]);

  async function logout() {
    try {
      await api('/auth/logout', { method: 'POST' });
    } catch {
      /* token may already be invalid */
    }
    setToken(null);
    setSession(null);
  }

  if (session === undefined) return <p className="muted center">Loading…</p>;
  if (session === null) return <AuthScreen onAuth={setSession} />;

  return (
    <div className="app">
      <header>
        <h1>🚕 MyCab</h1>
        <div>
          <span className="muted">
            {session.profile.name} ({session.role})
          </span>{' '}
          <button onClick={logout}>Log out</button>
        </div>
      </header>
      {session.role === 'driver' ? <DriverApp profile={session.profile} /> : <RiderApp />}
    </div>
  );
}

export default App;
