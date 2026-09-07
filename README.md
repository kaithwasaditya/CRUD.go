# Task API

A small Go CRUD API that stores tasks in PostgreSQL.

## Start the stack

1. Copy `.env.example` to `.env` and change the values if needed.
2. Run:

   ```sh
   docker compose up --build
   ```

The API is available at `http://localhost:8081`.

## Routes

- `GET /tasks`
- `GET /tasks/{id}`
- `POST /tasks` with `{ "title": "Buy milk" }`
- `PUT /tasks/{id}` with `{ "title": "Buy milk", "done": true }`
- `DELETE /tasks/{id}`

## Database

PostgreSQL runs in Docker. The connection string comes from `DATABASE_URL` in `.env`; `.env` is ignored by Git and `.env.example` is committed as a template.

The `db/init.sql` file creates the `tasks` table and inserts three sample tasks when the database volume is first created. The named `postgres_data` volume keeps data after the app or database container restarts.

Only the repository changed when moving from in-memory storage to PostgreSQL. The API routes and their request and response behavior stayed the same because they depend on the `TaskRepository` interface.

## Persistence check

I checked persistence by creating a task with `POST /tasks`, running `docker compose restart`, and then calling `GET /tasks`. The created task was still returned because it was stored in the `postgres_data` Docker volume.
