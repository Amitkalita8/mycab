import { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';

// Emoji pins. Rider sits above its point and the driver below, so they stay visible when at the same spot.
const pin = (emoji, color, anchorY) =>
  L.divIcon({
    className: 'pin',
    html: `<span style="border-color:${color}">${emoji}</span>`,
    iconSize: [32, 32],
    iconAnchor: [16, anchorY],
  });

// Read-only map of one ride: the rider's pickup, the drop-off, and the driver's live
// position once one is assigned.
export default function RideMap({ ride }) {
  const el = useRef(null);
  const state = useRef({});

  useEffect(() => {
    const m = L.map(el.current, { scrollWheelZoom: false });
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(m);
    state.current = {
      map: m,
      route: L.polyline([], { color: '#111827', weight: 3, dashArray: '6 6' }).addTo(m),
      pickup: L.marker([0, 0], { icon: pin('🧍', '#16a34a', 36), zIndexOffset: 1000 }).addTo(m),
      dropoff: L.marker([0, 0], { icon: pin('🏁', '#dc2626', 36) }).addTo(m),
      driver: L.marker([0, 0], { icon: pin('🚕', '#2563eb', -4) }),
      fitted: false,
    };
    return () => {
      m.stop(); // cancel any in-flight pan/zoom so it can't fire after removal
      m.remove();
    };
  }, []);

  useEffect(() => {
    const s = state.current;
    const p = [ride.pickup_lat, ride.pickup_lng];
    const d = [ride.dropoff_lat, ride.dropoff_lng];
    s.pickup.setLatLng(p).bindTooltip(`Rider pickup: ${ride.pickup}`);
    s.dropoff.setLatLng(d).bindTooltip(`Drop-off: ${ride.dropoff}`);
    s.route.setLatLngs([p, d]);

    const points = [p, d];
    const showDriver = ride.driver_lat != null && ['accepted', 'started'].includes(ride.status);
    if (showDriver) {
      const pos = [ride.driver_lat, ride.driver_lng];
      s.driver.setLatLng(pos).bindTooltip(`Driver: ${ride.driver_name || 'you'}`).addTo(s.map);
      points.push(pos);
    } else {
      s.driver.remove();
    }
    // fit once, then only re-fit if something moves out of view (don't fight the user's panning)
    const bounds = L.latLngBounds(points);
    if (!s.fitted || !s.map.getBounds().contains(bounds)) {
      s.map.fitBounds(bounds, { padding: [30, 30], maxZoom: 16, animate: false });
      s.fitted = true;
    }
  }, [ride]);

  return (
    <>
      <div ref={el} className="map small" data-testid="ride-map" />
      <p className="muted legend">🧍 Rider pickup · 🚕 Driver · 🏁 Drop-off</p>
    </>
  );
}
