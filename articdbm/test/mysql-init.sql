-- ArticDBM MySQL Test Database Init
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'viewer',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS products (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    category VARCHAR(100),
    price DECIMAL(10,2),
    stock INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orders (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT,
    product_id INT,
    quantity INT DEFAULT 1,
    total DECIMAL(10,2),
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);

INSERT INTO users (username, email, role) VALUES
    ('testadmin', 'admin@test.local', 'admin'),
    ('testuser1', 'user1@test.local', 'maintainer'),
    ('testuser2', 'user2@test.local', 'viewer'),
    ('testuser3', 'user3@test.local', 'viewer');

INSERT INTO products (name, category, price, stock) VALUES
    ('Widget Alpha', 'hardware', 29.99, 150),
    ('Service Beta', 'software', 49.99, 999),
    ('Gadget Gamma', 'hardware', 19.99, 75),
    ('License Delta', 'software', 99.99, 500);

INSERT INTO orders (user_id, product_id, quantity, total, status) VALUES
    (1, 1, 2, 59.98, 'completed'),
    (2, 2, 1, 49.99, 'completed'),
    (3, 3, 3, 59.97, 'pending'),
    (1, 4, 1, 99.99, 'processing');
