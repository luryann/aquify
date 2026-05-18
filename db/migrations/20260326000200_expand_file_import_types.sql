-- +goose Up
ALTER TABLE file_imports
  DROP CONSTRAINT IF EXISTS file_imports_file_type_check;

ALTER TABLE file_imports
  ADD CONSTRAINT file_imports_file_type_check CHECK (
    file_type IN ('cl2','sd3','zip','hy3','hyv','ev3','unknown')
  );

-- +goose Down
ALTER TABLE file_imports
  DROP CONSTRAINT IF EXISTS file_imports_file_type_check;

ALTER TABLE file_imports
  ADD CONSTRAINT file_imports_file_type_check CHECK (
    file_type IN ('cl2','sd3','zip','unknown')
  );
