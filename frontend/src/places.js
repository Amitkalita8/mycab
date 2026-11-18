// Demo locations (Bengaluru): quick picks alongside the map.
export const PLACES = [
  { name: 'MG Road', lat: 12.9756, lng: 77.6068 },
  { name: 'Indiranagar', lat: 12.9784, lng: 77.6408 },
  { name: 'Koramangala', lat: 12.9352, lng: 77.6245 },
  { name: 'Whitefield', lat: 12.9698, lng: 77.7499 },
  { name: 'Electronic City', lat: 12.8452, lng: 77.6602 },
  { name: 'Majestic Bus Stand', lat: 12.9767, lng: 77.5713 },
  { name: 'Kempegowda Airport', lat: 13.1986, lng: 77.7066 },
  { name: 'HSR Layout', lat: 12.9116, lng: 77.6389 },
];

export const coordsLabel = (lat, lng) => `${lat.toFixed(4)}, ${lng.toFixed(4)}`;

const geoCache = new Map();

// Turns coordinates into a readable place name using OpenStreetMap Nominatim (free, no key).
// This sends the coordinates to nominatim.openstreetmap.org. Falls back to "lat, lng" on any failure.
export async function reverseGeocode(lat, lng) {
  const key = `${lat.toFixed(4)},${lng.toFixed(4)}`;
  if (geoCache.has(key)) return geoCache.get(key);
  let name = coordsLabel(lat, lng);
  try {
    const res = await fetch(
      `https://nominatim.openstreetmap.org/reverse?format=jsonv2&zoom=18&accept-language=en&lat=${lat}&lon=${lng}`
    );
    if (res.ok) {
      const d = await res.json();
      const a = d.address || {};
      const street = a.road || a.pedestrian || a.amenity || a.building || a.neighbourhood;
      const area = a.suburb || a.neighbourhood || a.city_district || a.city || a.town || a.village;
      const parts = [...new Set([street, area].filter(Boolean))];
      if (parts.length) name = parts.join(', ');
      else if (d.display_name) name = d.display_name.split(',').slice(0, 2).join(',').trim();
    }
  } catch {
    /* offline or rate-limited: keep the coordinates label */
  }
  geoCache.set(key, name);
  return name;
}

export function currentPosition() {
  return new Promise((resolve, reject) => {
    if (!navigator.geolocation) return reject(new Error('Location is not available in this browser'));
    navigator.geolocation.getCurrentPosition(
      (p) => resolve({ lat: p.coords.latitude, lng: p.coords.longitude }),
      () => reject(new Error('Could not get your location (permission denied?)')),
      { enableHighAccuracy: true, timeout: 10000 }
    );
  });
}

export const money = (n) => `₹${Number(n).toFixed(2)}`;
