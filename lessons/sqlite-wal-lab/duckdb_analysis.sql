create table measurements as
select * from read_csv_auto('measurements.csv');

select * from measurements;
select 
  mode,
  round(write_ms, 2) as write_ms,
  result
from measurements
order by write_ms desc;

with m as (
  select * from measurements
)
select
  max(write_ms) - min(write_ms) as latency_gap_ms
from m;
