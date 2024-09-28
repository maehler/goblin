CREATE TABLE temperature (
    time TEXT PRIMARY KEY NOT NULL,
    sensor_id TEXT NOT NULL REFERENCES sensors(id),
    value FLOAT
);

CREATE TABLE humidity (
    time TEXT PRIMARY KEY NOT NULL,
    sensor_id TEXT NOT NULL REFERENCES sensors(id),
    value FLOAT
);

CREATE TABLE sensors (
    id TEXT PRIMARY KEY NOT NULL,
    sensor_type TEXT NOT NULL,
    room_id TEXT NOT NULL REFERENCES rooms(id)
);

CREATE TABLE rooms (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL
);
