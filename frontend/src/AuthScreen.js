import { useState } from 'react';
import { api, setToken } from './api';

export default function AuthScreen({ onAuth }) {
  const [role, setRole] = useState('rider');
  const [mode, setMode] = useState('login');
  const [form, setForm] = useState({
    name: '',
    phone: '',
    password: '',
    vehicle_model: '',
    vehicle_plate: '',
  });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value });

  async function submit(e) {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      const path = mode === 'login' ? '/auth/login' : `/auth/register/${role}`;
      const body =
        mode === 'login'
          ? { role, phone: form.phone, password: form.password }
          : role === 'rider'
          ? { name: form.name, phone: form.phone, password: form.password }
          : form;
      const data = await api(path, { method: 'POST', body });
      setToken(data.token);
      onAuth({ role: data.role, profile: data.profile });
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth card">
      <h1>🚕 MyCab</h1>
      <div className="tabs" role="tablist">
        {['rider', 'driver'].map((r) => (
          <button
            key={r}
            type="button"
            className={role === r ? 'active' : ''}
            onClick={() => setRole(r)}
          >
            {r === 'rider' ? 'I need a ride' : 'I drive'}
          </button>
        ))}
      </div>
      <form onSubmit={submit}>
        {mode === 'register' && (
          <label>
            Name
            <input value={form.name} onChange={set('name')} required />
          </label>
        )}
        <label>
          Phone
          <input value={form.phone} onChange={set('phone')} required inputMode="tel" />
        </label>
        <label>
          Password
          <input
            type="password"
            value={form.password}
            onChange={set('password')}
            required
            minLength={6}
            maxLength={72}
          />
        </label>
        {mode === 'register' && role === 'driver' && (
          <>
            <label>
              Vehicle model
              <input value={form.vehicle_model} onChange={set('vehicle_model')} required />
            </label>
            <label>
              Number plate
              <input value={form.vehicle_plate} onChange={set('vehicle_plate')} required />
            </label>
          </>
        )}
        {error && <p className="error">{error}</p>}
        <button className="primary" disabled={busy}>
          {busy ? 'Please wait…' : mode === 'login' ? 'Log in' : 'Create account'}
        </button>
      </form>
      <p className="muted">
        {mode === 'login' ? 'New here?' : 'Already registered?'}{' '}
        <button
          type="button"
          className="link"
          onClick={() => {
            setMode(mode === 'login' ? 'register' : 'login');
            setError('');
          }}
        >
          {mode === 'login' ? 'Create an account' : 'Log in'}
        </button>
      </p>
    </div>
  );
}
