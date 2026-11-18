import { useEffect, useRef, useState } from 'react';
import { api } from './api';
import { usePolling } from './hooks';
import { PLACES, reverseGeocode } from './places';
import RideCard from './RideCard';

const ACTIVE = ['accepted', 'started'];

export default function DriverApp({ profile: initial }) {
  const [me, setMe] = useState(initial);
  const [open, setOpen] = useState([]);
  const [mine, setMine] = useState([]);
  const [error, setError] = useState('');
  const lastSent = useRef(0);
  const [placeName, setPlaceName] = useState('');

  const load = async () => {
    try {
      const [m, rides, requests] = await Promise.all([
        api('/me'),
        api('/rides'),
        api('/rides?scope=open'),
      ]);
      setMe(m.profile);
      setMine(rides);
      setOpen(requests);
    } catch (e) {
      setError(e.message);
    }
  };
  usePolling(load, 4000);

  useEffect(() => {
    if (me.lat == null) return;
    let live = true;
    reverseGeocode(me.lat, me.lng).then((n) => live && setPlaceName(n));
    return () => {
      live = false;
    };
  }, [me.lat, me.lng]);

  const active = mine.find((r) => ACTIVE.includes(r.status));

  async function setAvailable(available) {
    try {
      setMe(await api('/driver/availability', { method: 'PATCH', body: { available } }));
    } catch (e) {
      setError(e.message);
    }
  }

  async function sendLocation(lat, lng) {
    try {
      setMe(await api('/driver/location', { method: 'PUT', body: { lat, lng } }));
    } catch (e) {
      setError(e.message);
    }
  }

  // While online or on a ride, stream the device location (at most every 10s).
  const sharing = me.available || !!active;
  useEffect(() => {
    if (!sharing || !navigator.geolocation) return;
    const id = navigator.geolocation.watchPosition(
      (p) => {
        if (Date.now() - lastSent.current < 10000) return;
        lastSent.current = Date.now();
        sendLocation(p.coords.latitude, p.coords.longitude);
      },
      () => {}, // no GPS (e.g. desktop): use the manual location picker below
      { enableHighAccuracy: true }
    );
    return () => navigator.geolocation.clearWatch(id);
  }, [sharing]); // eslint-disable-line react-hooks/exhaustive-deps

  const history = mine.filter((r) => r !== active);

  return (
    <>
      {error && (
        <p className="error" role="alert" onClick={() => setError('')}>
          {error}
        </p>
      )}

      <div className="card">
        <div className="row">
          <div>
            <strong>{me.name}</strong>
            <div className="muted">
              {me.vehicle_model} · {me.vehicle_plate} ·{' '}
              {me.rating != null ? `★ ${me.rating.toFixed(1)} (${me.rating_count})` : 'no ratings yet'}
            </div>
          </div>
          <button
            className={me.available ? 'danger' : 'primary'}
            disabled={!me.available && !!active}
            onClick={() => setAvailable(!me.available)}
          >
            {me.available ? 'Go offline' : 'Go online'}
          </button>
        </div>
        <label>
          My location {me.lat != null && <span className="muted">— {placeName || 'locating…'} ({me.lat.toFixed(4)}, {me.lng.toFixed(4)})</span>}
          <select
            value=""
            onChange={(e) => {
              const p = PLACES.find((x) => x.name === e.target.value);
              if (p) sendLocation(p.lat, p.lng);
            }}
          >
            <option value="">Set manually…</option>
            {PLACES.map((p) => (
              <option key={p.name}>{p.name}</option>
            ))}
          </select>
        </label>
      </div>

      {active && (
        <>
          <h2>Current ride</h2>
          <RideCard ride={active} role="driver" onChange={load} onError={setError} />
        </>
      )}

      {!active && me.available && (
        <>
          <h2>Ride requests</h2>
          {open.length === 0 && <p className="muted">Waiting for requests…</p>}
          {open.map((r) => (
            <RideCard key={r.id} ride={r} role="driver" onChange={load} onError={setError} />
          ))}
        </>
      )}
      {!active && !me.available && <p className="muted">Go online to receive ride requests.</p>}

      {history.length > 0 && <h2>Past rides</h2>}
      {history.map((r) => (
        <RideCard key={r.id} ride={r} role="driver" onChange={load} onError={setError} />
      ))}
    </>
  );
}
