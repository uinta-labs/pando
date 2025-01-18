
-- name: GetCurrentScheduleForDevice :one
SELECT s.*
FROM device AS d
LEFT JOIN fleet_schedule AS fs ON fs.fleet_id = d.fleet_id
LEFT JOIN fleet AS f ON f.id = fs.fleet_id
LEFT JOIN schedule AS s ON s.id = f.default_schedule_id
WHERE d.id = pggen.arg('device_id');

-- name: GetContainersForSchedule :many
SELECT c.*
FROM container AS c
WHERE c.schedule_id = pggen.arg('schedule_id');

-- name: GetDeviceByName :one
SELECT d.*
FROM device AS d
WHERE d.name = pggen.arg('name');