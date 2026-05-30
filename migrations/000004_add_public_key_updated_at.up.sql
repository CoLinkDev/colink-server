ALTER TABLE devices
ADD COLUMN public_key_updated_at timestamptz NOT NULL DEFAULT now();
