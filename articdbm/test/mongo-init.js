// ArticDBM MongoDB Test Database Init
db = db.getSiblingDB('testdb');

db.createUser({
    user: 'testuser',
    pwd: 'testpass123',
    roles: [
        { role: 'readWrite', db: 'testdb' }
    ]
});

db.users.insertMany([
    { username: 'testadmin', email: 'admin@test.local', role: 'admin', createdAt: new Date() },
    { username: 'testuser1', email: 'user1@test.local', role: 'maintainer', createdAt: new Date() },
    { username: 'testuser2', email: 'user2@test.local', role: 'viewer', createdAt: new Date() },
    { username: 'testuser3', email: 'user3@test.local', role: 'viewer', createdAt: new Date() }
]);

db.products.insertMany([
    { name: 'Widget Alpha', category: 'hardware', price: 29.99, stock: 150, createdAt: new Date() },
    { name: 'Service Beta', category: 'software', price: 49.99, stock: 999, createdAt: new Date() },
    { name: 'Gadget Gamma', category: 'hardware', price: 19.99, stock: 75, createdAt: new Date() },
    { name: 'License Delta', category: 'software', price: 99.99, stock: 500, createdAt: new Date() }
]);

db.orders.insertMany([
    { userId: 'testadmin', productName: 'Widget Alpha', quantity: 2, total: 59.98, status: 'completed', createdAt: new Date() },
    { userId: 'testuser1', productName: 'Service Beta', quantity: 1, total: 49.99, status: 'completed', createdAt: new Date() },
    { userId: 'testuser2', productName: 'Gadget Gamma', quantity: 3, total: 59.97, status: 'pending', createdAt: new Date() },
    { userId: 'testadmin', productName: 'License Delta', quantity: 1, total: 99.99, status: 'processing', createdAt: new Date() }
]);
