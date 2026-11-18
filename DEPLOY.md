# Deploying MyCab for free (Render + Turso)

One Render web service runs the Go API **and** serves the React app, so there is a single URL
and no CORS setup. Data lives in a free hosted Turso (SQLite) database, because Render's free
tier has no persistent disk and sleeps when idle.

Free-plan limits change — check each provider's pricing page.

## 1. Create the database (Turso)
1. Sign up at https://turso.tech and create a database (any name, nearest region).
2. Open the database in the dashboard: copy its **URL** (`libsql://<name>-<org>.turso.io`)
   and create a **token** (read & write).
3. Your connection string is:
   `libsql://<name>-<org>.turso.io?authToken=<token>`
   Treat it like a password. Tables are created automatically on first start.

## 2. Push the code to GitHub
Commit and publish the repo (the `Dockerfile` and `render.yaml` are in the repo root).

## 3. Create the service (Render)
1. https://render.com → **New → Blueprint** → pick your GitHub repo (it reads `render.yaml`).
   (Or **New → Web Service**, runtime **Docker**, instance type **Free**.)
2. When asked, set the env var **DATABASE_URL** to the connection string from step 1.
3. Deploy. The first build takes several minutes (it builds the frontend and backend).
4. Open the `https://<name>.onrender.com` URL.

## Notes
- **Cold starts:** the free service sleeps after ~15 min idle; the first request then takes
  30–60 s. Data is safe in Turso.
- **Geolocation** works because Render serves HTTPS.
- **Payments are simulated**; there is no real payment gateway.
- **Local development** still uses a local SQLite file; set `DATABASE_URL` only to test against Turso.
- Optional env vars: `PORT` (Render sets it), `ALLOWED_ORIGIN` (only if the frontend is hosted elsewhere),
  `STATIC_DIR` (set by the Dockerfile).
