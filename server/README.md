# dba

Monitor whether a database is online and how much space it is using.

Gin serves the API and the built web UI. Targets and hourly samples live in SQLite (`data/stats.db`). Login requires the Access Key plus a 6-digit TOTP code; the first start writes both once to `data/bootstrap.txt`. Add a database with a full URL, including query parameters such as `ssl-mode`, charset, and timeouts. Space is sampled every hour and can be refreshed from the UI.
