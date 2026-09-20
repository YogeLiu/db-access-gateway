CREATE DATABASE IF NOT EXISTS database_a;
CREATE DATABASE IF NOT EXISTS database_b;
CREATE DATABASE IF NOT EXISTS database_c;

CREATE TABLE database_a.orders (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  customer VARCHAR(80) NOT NULL,
  amount DECIMAL(12,2) NOT NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'pending'
);
INSERT INTO database_a.orders(customer, amount, status) VALUES
  ('Ada', 128.50, 'paid'), ('Linus', 42.00, 'pending');

CREATE TABLE database_b.inventory (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  sku VARCHAR(64) NOT NULL UNIQUE,
  quantity INT NOT NULL
);
INSERT INTO database_b.inventory(sku, quantity) VALUES ('DB-GATE-001', 20);

CREATE TABLE database_c.notes (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  body VARCHAR(255) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO database_c.notes(body) VALUES ('write access demo');

CREATE USER IF NOT EXISTS 'gateway_read'@'%' IDENTIFIED BY 'demo-read-password';
CREATE USER IF NOT EXISTS 'gateway_write'@'%' IDENTIFIED BY 'demo-write-password';
GRANT SELECT, SHOW VIEW ON database_a.* TO 'gateway_read'@'%';
GRANT SELECT, SHOW VIEW ON database_b.* TO 'gateway_read'@'%';
GRANT SELECT, SHOW VIEW ON database_c.* TO 'gateway_read'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON database_a.* TO 'gateway_write'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON database_b.* TO 'gateway_write'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON database_c.* TO 'gateway_write'@'%';

