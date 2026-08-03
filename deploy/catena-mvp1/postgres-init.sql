\set ON_ERROR_STOP on

CREATE USER langwatch WITH PASSWORD 'langwatch-local';
CREATE DATABASE langwatch OWNER langwatch;

CREATE USER catena_core WITH PASSWORD 'catena-core-local';
CREATE DATABASE catena_core OWNER catena_core;
