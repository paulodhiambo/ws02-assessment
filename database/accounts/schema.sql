-- Jamii Savings – accounts schema (MySQL 8)
-- Loaded automatically by the MySQL container on first start
-- (mounted into /docker-entrypoint-initdb.d as 01-schema.sql).

CREATE TABLE IF NOT EXISTS customers (
    customer_id     VARCHAR(20)  NOT NULL,
    full_name       VARCHAR(120) NOT NULL,
    national_id     VARCHAR(20)  NOT NULL,
    phone_number    VARCHAR(20)  NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT pk_customers PRIMARY KEY (customer_id),
    CONSTRAINT uq_customers_national_id UNIQUE (national_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE TABLE IF NOT EXISTS accounts (
    account_number  CHAR(10)      NOT NULL,
    customer_id     VARCHAR(20)   NOT NULL,
    account_type    VARCHAR(20)   NOT NULL,
    status          VARCHAR(10)   NOT NULL,
    balance         DECIMAL(18,2) NOT NULL DEFAULT 0.00,
    currency        CHAR(3)       NOT NULL DEFAULT 'KES',
    opened_on       DATE          NOT NULL,
    updated_at      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT pk_accounts PRIMARY KEY (account_number),
    CONSTRAINT fk_accounts_customer FOREIGN KEY (customer_id) REFERENCES customers (customer_id),
    CONSTRAINT ck_accounts_status CHECK (status IN ('ACTIVE', 'DORMANT', 'CLOSED')),
    CONSTRAINT ck_accounts_number CHECK (account_number REGEXP '^[0-9]{10}$')
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

CREATE INDEX ix_accounts_customer ON accounts (customer_id);
