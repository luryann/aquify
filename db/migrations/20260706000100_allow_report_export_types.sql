-- +goose Up
-- file_exports now stores the server-generated PDF meet reports
-- (eligibility/entry/event). The original constraint only allowed the SD3
-- variants, so every report row was rejected and reports existed only in the
-- generating process's memory (lost on restart, invisible to other instances).
ALTER TABLE file_exports DROP CONSTRAINT IF EXISTS file_exports_export_type_check;
ALTER TABLE file_exports ADD CONSTRAINT file_exports_export_type_check CHECK (
  export_type IN (
    'standard_sd3', 'extended_sd3', 'sdif_roster',
    'eligibility_report', 'entry_report', 'event_report'
  )
);

-- +goose Down
DELETE FROM file_exports WHERE export_type IN ('eligibility_report', 'entry_report', 'event_report');
ALTER TABLE file_exports DROP CONSTRAINT IF EXISTS file_exports_export_type_check;
ALTER TABLE file_exports ADD CONSTRAINT file_exports_export_type_check CHECK (
  export_type IN ('standard_sd3', 'extended_sd3', 'sdif_roster')
);
