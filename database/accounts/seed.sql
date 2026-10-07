-- Jamii Savings – seed data (fictional). Loaded after schema.sql.

INSERT INTO customers (customer_id, full_name, national_id, phone_number) VALUES
    ('1', 'Leanne Graham',    '20000001', '+254700000001'),
    ('2', 'Ervin Howell',     '20000002', '+254700000002'),
    ('3', 'Clementine Bauch', '20000003', '+254700000003'),
    ('4', 'Patricia Lebsack', '20000004', '+254700000004');

INSERT INTO accounts (account_number, customer_id, account_type, status, balance, currency, opened_on) VALUES
    ('0100000001', '1', 'SAVINGS', 'ACTIVE',  152340.75, 'KES', '2019-03-14'),
    ('0100000002', '1', 'CURRENT', 'ACTIVE',    8450.00, 'KES', '2021-07-01'),
    ('0100000003', '2', 'SAVINGS', 'DORMANT',   1200.50, 'KES', '2016-11-20'),
    ('0100000004', '3', 'SAVINGS', 'CLOSED',       0.00, 'KES', '2015-02-02'),
    ('0100000005', '4', 'FIXED',   'ACTIVE', 1000000.00, 'KES', '2024-01-10'),
    ('0100000006', '4', 'SAVINGS', 'ACTIVE',     310.40, 'USD', '2023-05-05');
