import { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';

const dot = (color, radius) => ({
  radius,
  color: '#fff',
  weight: 2,
  fillColor: color,
  fillOpacity: 1,
});

// OpenStreetMap map. Clicking calls onPick(lat, lng); markers show pickup (green),
// drop-off (red) and nearby drivers (yellow).
export default function MapPicker({ pickup, dropoff, drivers = [], onPick }) {
  const el = useRef(null);
  const map = useRef(null);
  const layers = useRef({});
  const onPickRef = useRef(onPick);
  onPickRef.current = onPick;

  useEffect(() => {
    const m = L.map(el.current).setView([pickup.lat, pickup.lng], 12);
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(m);
    m.on('click', (e) => onPickRef.current(e.latlng.lat, e.latlng.lng));
    layers.current = {
      route: L.polyline([], { color: '#111827', weight: 3, dashArray: '6 6' }).addTo(m),
      pickup: L.circleMarker([pickup.lat, pickup.lng], dot('#16a34a', 9)).addTo(m),
      dropoff: L.circleMarker([dropoff.lat, dropoff.lng], dot('#dc2626', 9)).addTo(m),
      drivers: L.layerGroup().addTo(m),
    };
    map.current = m;
    m.fitBounds([[pickup.lat, pickup.lng], [dropoff.lat, dropoff.lng]], { padding: [30, 30], animate: false });
    return () => {
      m.stop(); // cancel any in-flight pan/zoom so it can't fire after removal
      m.remove();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const m = map.current;
    const { route, pickup: pm, dropoff: dm } = layers.current;
    pm.setLatLng([pickup.lat, pickup.lng]).bindTooltip(`Pickup: ${pickup.name}`);
    dm.setLatLng([dropoff.lat, dropoff.lng]).bindTooltip(`Drop-off: ${dropoff.name}`);
    route.setLatLngs([[pickup.lat, pickup.lng], [dropoff.lat, dropoff.lng]]);
    // keep both points visible without jumping the map around on every click
    const bounds = L.latLngBounds([[pickup.lat, pickup.lng], [dropoff.lat, dropoff.lng]]);
    if (!m.getBounds().contains(bounds)) m.fitBounds(bounds, { padding: [30, 30], animate: false });
  }, [pickup, dropoff]);

  useEffect(() => {
    const g = layers.current.drivers;
    g.clearLayers();
    drivers.forEach((d) =>
      L.circleMarker([d.lat, d.lng], dot('#f5b800', 6))
        .bindTooltip(`${d.name} · ${d.vehicle_model}`)
        .addTo(g)
    );
  }, [drivers]);

  return <div ref={el} className="map" data-testid="map" />;
}
