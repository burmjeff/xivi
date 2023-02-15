import { w as writable } from "./index2.js";
class OrderLine {
  constructor(product, quantity) {
    this.product = product;
    this.quantity = quantity;
  }
  get total() {
    return this.product.price * this.quantity;
  }
}
class Order {
  lines = /* @__PURE__ */ new Map();
  constructor(initialLines) {
    if (initialLines)
      initialLines.forEach((ol) => this.lines.set(ol.product.id, ol));
  }
  addProduct(prod, quantity) {
    if (this.lines.has(prod.id)) {
      if (quantity === 0) {
        this.removeProduct(prod.id);
      } else {
        const orderLine = this.lines.get(prod.id);
        if (orderLine)
          orderLine.quantity += quantity;
      }
    } else {
      this.lines.set(prod.id, new OrderLine(prod, quantity));
    }
  }
  removeProduct(id) {
    this.lines.delete(id);
  }
  get orderLines() {
    return [...this.lines.values()];
  }
  get productCount() {
    return [...this.lines.values()].reduce((total, ol) => total += ol.quantity, 0);
  }
  get total() {
    return [...this.lines.values()].reduce((total, ol) => total += ol.total, 0);
  }
}
const products = writable([]);
const selectedCategory = writable("");
const order = writable(new Order());
export {
  Order as O,
  order as o,
  products as p,
  selectedCategory as s
};
