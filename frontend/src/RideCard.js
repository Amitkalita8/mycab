import { useState } from 'react';
import { api } from './api';
import { money } from './places';
import RideMap from './RideMap';

const LABELS = {
  requested: 'Looking for a driver…',
  accepted: 'Driver on the way',
  started: 'Trip in progress',
  completed: 'Completed',
  cancelled: 'Cancelled',
};

// One ride, with the actions the current role may take. `onChange` re-fetches after an action.
export default function RideCard({ ride, role, onChange, onError }) {
  const [rating, setRating] = useState(5);
  const [review, setReview] = useState('');
  const [method, setMethod] = useState(ride.payment_method);

  async function act(path, body) {
    try {
      await api(`/rides/${ride.id}/${path}`, { method: 'POST', body });
      onChange();
    } catch (err) {
      onError(err.message);
    }
  }

  const rider = role === 'rider';
  const hasDriver = ride.driver_id != null;

  return (
    <div className={`card ride ${ride.status}`}>
      <div className="row">
        <strong>
          {ride.pickup} → {ride.dropoff}
        </strong>
        <span className={`badge ${ride.status}`}>{LABELS[ride.status]}</span>
      </div>
      <div className="muted">
        {ride.distance_km.toFixed(1)} km · {money(ride.fare)} · {ride.payment_method}
        {ride.status === 'completed' && ` (${ride.payment_status})`}
      </div>

      {rider && hasDriver && ride.status !== 'cancelled' && (
        <p>
          Driver: <strong>{ride.driver_name}</strong> · {ride.driver_vehicle} ({ride.driver_plate}) ·{' '}
          {ride.driver_phone}
        </p>
      )}
      {!rider && <p className="muted">Rider: {ride.rider_name}</p>}

      {/* route + live driver position; shown while the ride is open or in progress */}
      {['requested', 'accepted', 'started'].includes(ride.status) && <RideMap ride={ride} />}

      <div className="actions">
        {role === 'driver' && ride.status === 'requested' && (
          <button className="primary" onClick={() => act('accept')}>
            Accept ride
          </button>
        )}
        {role === 'driver' && ride.status === 'accepted' && (
          <button className="primary" onClick={() => act('start')}>
            Start trip
          </button>
        )}
        {role === 'driver' && ride.status === 'started' && (
          <button className="primary" onClick={() => act('complete')}>
            Complete trip
          </button>
        )}
        {((rider && ['requested', 'accepted'].includes(ride.status)) ||
          (!rider && ride.status === 'accepted')) && (
          <button className="danger" onClick={() => act('cancel')}>
            Cancel
          </button>
        )}
      </div>

      {rider && ride.status === 'completed' && ride.payment_status === 'pending' && (
        <div className="actions">
          <select value={method} onChange={(e) => setMethod(e.target.value)} aria-label="Payment method">
            <option value="cash">Cash</option>
            <option value="card">Card</option>
            <option value="wallet">Wallet</option>
          </select>
          <button className="primary" onClick={() => act('pay', { method })}>
            Pay {money(ride.fare)}
          </button>
        </div>
      )}

      {rider && ride.status === 'completed' && hasDriver && ride.rating == null && (
        <div className="actions">
          <select value={rating} onChange={(e) => setRating(Number(e.target.value))} aria-label="Rating">
            {[5, 4, 3, 2, 1].map((n) => (
              <option key={n} value={n}>
                {'★'.repeat(n)}
              </option>
            ))}
          </select>
          <input
            placeholder="Review (optional)"
            value={review}
            maxLength={500}
            onChange={(e) => setReview(e.target.value)}
          />
          <button onClick={() => act('rate', { rating, review })}>Rate driver</button>
        </div>
      )}
      {ride.rating != null && (
        <p className="muted">
          Rated {'★'.repeat(ride.rating)}
          {ride.review ? ` — “${ride.review}”` : ''}
        </p>
      )}
    </div>
  );
}
