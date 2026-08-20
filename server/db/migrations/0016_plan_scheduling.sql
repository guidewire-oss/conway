-- +goose Up
-- Plan-level scheduling policy for the execution-order scheduler (spec 001 §7
-- SchedulingParams): period start, WIP limits, buffer percentages, lead capacity
-- and the calendar. Stored as one jsonb blob rather than a column per knob
-- because §7 is still growing — feeding buffers, transfers and calendar windows
-- all land here — and because nothing queries inside it: it is read whole,
-- handed to ComputeSchedule, and written whole.
--
-- Nullable, like teams and initiatives in this same table, rather than
-- NOT NULL DEFAULT '{}': jsonbArg turns a nil slice into SQL NULL, so a
-- NOT NULL column would add a runtime failure mode for any future caller
-- without buying anything — planScheduling already reads an absent or empty
-- blob as "no policy set", which schedules exactly as a plan with no policy
-- at all does (FR-002).
ALTER TABLE plans ADD COLUMN IF NOT EXISTS scheduling jsonb;

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS scheduling;
