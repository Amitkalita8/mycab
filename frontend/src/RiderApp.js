import { useEffect, useState } from 'react';
import { api } from './api';
import { usePolling } from './hooks';
import MapPicker from './MapPicker';
import { PLACES, coordsLabel, currentPosition, money, reverseGeocode } from './places';
import RideCard from './RideCard';

const ACTIVE = ['requested', 'accepted', 'started'];

export default function RiderApp() {
  const [rides, setRides] = useState([]);
  const [pickup, setPickup] = useState(PLACES[0]);
  const [dropoff, setDropoff] = useState(PLACES[1]);
  const [method, setMethod] = useState('cash');
  const [estimate, setEstimate] = useState(null);
  const [nearby, setNearby] = useState([]);
  const [target, setTarget] = useState('dropoff'); // what a map click sets
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = () =>
    api('/rides')
      .then(setRides)
      .catch((e) => setError(e.message));

  usePolling(load, 4000);

  const active = rides.find((r) => ACTIVE.includes(r.status));

  // fare estimate whenever the route changes
  useEffect(() => {
    let live = true;
    api('/fare-estimate', {
      method: 'POST',
      body: {
        pickup_lat: pickup.lat,
        pickup_lng: pickup.lng,
        dropoff_lat: dropoff.lat,
        dropoff_lng: dropoff.lng,
      },
    })
      .then((d) => live && setEstimate(d))
      .catch(() => live && setEstimate(null));
    return () => {
      live = false;
    };
  }, [pickup, dropoff]);

  // drivers near the pickup point: refresh every 8s and whenever the pickup changes
  const loadNearby = () => {
    if (active) return;
    api(`/drivers/nearby?lat=${pickup.lat}&lng=${pickup.lng}`)
      .then(setNearby)
      .catch(() => setNearby([]));
  };
  usePolling(loadNearby, 8000);
  useEffect(loadNearby, [pickup, active]); // eslint-disable-line react-hooks/exhaustive-deps

  async function book(e) {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      await api('/rides', {
        method: 'POST',
        body: {
          pickup: pickup.name,
          dropoff: dropoff.name,
          pickup_lat: pickup.lat,
          pickup_lng: pickup.lng,
          dropoff_lat: dropoff.lat,
          dropoff_lng: dropoff.lng,
          payment_method: method,
        },
      });
      await load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  // Set a point from raw coordinates, then replace the label with the real place name once looked up.
  function setPoint(which, lat, lng) {
    const setter = which === 'pickup' ? setPickup : setDropoff;
    setter({ name: coordsLabel(lat, lng), lat, lng });
    reverseGeocode(lat, lng).then((name) =>
      setter((p) => (p.lat === lat && p.lng === lng ? { ...p, name } : p))
    );
  }

  async function useMyLocation() {
    try {
      const { lat, lng } = await currentPosition();
      setPoint('pickup', lat, lng);
    } catch (err) {
      setError(err.message);
    }
  }

  const options = (sel) =>
    PLACES.some((p) => p.name === sel.name) ? PLACES : [sel, ...PLACES];
  const pick = (setter, list) => (e) => setter(list.find((p) => p.name === e.target.value));

  const history = rides.filter((r) => r !== active);

  return (
    <>
      {error && (
        <p className="error" role="alert" onClick={() => setError('')}>
          {error}
        </p>
      )}

      {active ? (
        <>
          <h2>Your current ride</h2>
          <RideCard ride={active} role="rider" onChange={load} onError={setError} />
        </>
      ) : (
        <form className="card" onSubmit={book}>
          <h2>Book a ride</h2>
          <label>
            Pickup
            <div className="row">
              <select value={pickup.name} onChange={pick(setPickup, options(pickup))}>
                {options(pickup).map((p) => (
                  <option key={p.name}>{p.name}</option>
                ))}
              </select>
              <button type="button" onClick={useMyLocation}>
                📍 Use mine
              </button>
            </div>
          </label>
          <label>
            Drop-off
            <select value={dropoff.name} onChange={pick(setDropoff, options(dropoff))}>
              {options(dropoff).map((p) => (
                <option key={p.name}>{p.name}</option>
              ))}
            </select>
          </label>
          <div className="tabs" role="group" aria-label="Map click sets">
            <button
              type="button"
              className={target === 'pickup' ? 'active' : ''}
              onClick={() => setTarget('pickup')}
            >
              🟢 Set pickup on map
            </button>
            <button
              type="button"
              className={target === 'dropoff' ? 'active' : ''}
              onClick={() => setTarget('dropoff')}
            >
              🔴 Set drop-off on map
            </button>
          </div>
          <MapPicker
            pickup={pickup}
            dropoff={dropoff}
            drivers={nearby}
            onPick={(lat, lng) => setPoint(target, lat, lng)}
          />
          <label>
            Payment
            <select value={method} onChange={(e) => setMethod(e.target.value)}>
              <option value="cash">Cash</option>
              <option value="card">Card</option>
              <option value="wallet">Wallet</option>
            </select>
          </label>
          {estimate && (
            <p>
              Estimated <strong>{money(estimate.fare)}</strong> · {estimate.distance_km.toFixed(1)} km
            </p>
          )}
          <p className="muted">
            {nearby.length
              ? `${nearby.length} driver${nearby.length > 1 ? 's' : ''} nearby — closest ${nearby[0].distance_km.toFixed(1)} km away`
              : 'No drivers nearby right now'}
          </p>
          <button className="primary" disabled={busy || (pickup.lat === dropoff.lat && pickup.lng === dropoff.lng)}>
            {busy ? 'Booking…' : 'Book ride'}
          </button>
        </form>
      )}

      {history.length > 0 && <h2>Past rides</h2>}
      {history.map((r) => (
        <RideCard key={r.id} ride={r} role="rider" onChange={load} onError={setError} />
      ))}
    </>
  );
}
