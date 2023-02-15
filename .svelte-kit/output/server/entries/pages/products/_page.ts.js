import Axios from "axios";
import { p as products, o as order, O as Order, s as selectedCategory } from "../../../chunks/stores2.js";
const urls = {
  products: "/api/books",
  orders: "/api/orders"
};
class HttpHandler {
  async loadProducts() {
    {
      const response = await Axios.get(urls.products);
      return response.data;
    }
  }
  async storeOrder(order2) {
    {
      const orderData = {
        lines: [...order2.orderLines.values()].map((ol) => ({
          productId: ol.product.id,
          productName: ol.product.name,
          quantity: ol.quantity
        }))
      };
      const response = await Axios.post(urls.orders, orderData);
      return response.data.id;
    }
  }
}
const load = async () => {
  products.set(await new HttpHandler().loadProducts());
  order.set(new Order());
  selectedCategory.set("All");
};
export {
  load
};
