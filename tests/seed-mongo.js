const demo = db.getSiblingDB('backup-ui-test');

demo.customers.insertMany([
  { _id: 1, name: 'Ada Lovelace', email: 'ada@example.test', plan: 'team' },
  { _id: 2, name: 'Grace Hopper', email: 'grace@example.test', plan: 'starter' },
  { _id: 3, name: 'Edsger Dijkstra', email: 'edsger@example.test', plan: 'team' },
]);

demo.products.insertMany([
  { _id: 'notebook', name: 'Field notebook', price: 12.5, stock: 80 },
  { _id: 'keyboard', name: 'Mechanical keyboard', price: 99, stock: 24 },
  { _id: 'mug', name: 'Coffee mug', price: 18, stock: 120 },
]);

demo.orders.insertMany(Array.from({ length: 40 }, (_, i) => ({
  _id: i + 1,
  customerId: (i % 3) + 1,
  productId: ['notebook', 'keyboard', 'mug'][i % 3],
  quantity: (i % 4) + 1,
  status: i % 5 === 0 ? 'pending' : 'fulfilled',
  createdAt: new Date(Date.UTC(2026, 0, 1, 0, i)),
})));

demo.events.insertMany(Array.from({ length: 100 }, (_, i) => ({
  type: ['page_view', 'checkout', 'sign_in'][i % 3],
  customerId: (i % 3) + 1,
  occurredAt: new Date(Date.UTC(2026, 0, 1, 0, i)),
})));

print('Seeded backup-ui-test with customers, products, orders, and events.');
