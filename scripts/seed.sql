-- Seed data for development/testing

-- Create a test tenant
INSERT INTO tenants (id, name, slug, plan, max_devices)
VALUES (
  '11111111-1111-1111-1111-111111111111',
  'Demo Fleet Company',
  'demo-fleet',
  'professional',
  1000
) ON CONFLICT DO NOTHING;

-- Create admin user (password: Admin@1234)
-- bcrypt hash of 'Admin@1234'
INSERT INTO users (id, tenant_id, email, password_hash, name, role)
VALUES (
  '22222222-2222-2222-2222-222222222222',
  '11111111-1111-1111-1111-111111111111',
  'admin@demo.com',
  '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
  'Admin User',
  'admin'
) ON CONFLICT DO NOTHING;

-- Create test vehicles
INSERT INTO vehicles (id, tenant_id, reg_number, make, model, year, fuel_type, color)
VALUES
  ('33333333-3333-3333-3333-333333333331', '11111111-1111-1111-1111-111111111111', 'KA-01-AB-1234', 'Tata', 'Ace', 2022, 'diesel', 'white'),
  ('33333333-3333-3333-3333-333333333332', '11111111-1111-1111-1111-111111111111', 'KA-01-CD-5678', 'Mahindra', 'Bolero', 2021, 'diesel', 'grey'),
  ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'KA-01-EF-9012', 'Tata', 'Prima', 2023, 'diesel', 'blue')
ON CONFLICT DO NOTHING;

-- Register GPS devices (IMEI must be registered before devices can connect)
INSERT INTO devices (id, tenant_id, vehicle_id, imei, model, sim_number, sim_operator, is_active)
VALUES
  ('44444444-4444-4444-4444-444444444441', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333331', '868523040000001', 'GT06N', '9876543210', 'airtel', true),
  ('44444444-4444-4444-4444-444444444442', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333332', '868523040000002', 'GT06N', '9876543211', 'airtel', true),
  ('44444444-4444-4444-4444-444444444443', '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', '868523040000003', 'GT06E', '9876543212', 'airtel', true)
ON CONFLICT DO NOTHING;

-- Pre-populate device info cache entries in Redis (run separately via redis-cli):
-- HSET device:868523040000001:info tenant_id 11111111-1111-1111-1111-111111111111 device_id 44444444-4444-4444-4444-444444444441 is_active true

SELECT 'Seed data inserted successfully' AS status;
