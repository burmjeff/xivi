import { c as create_ssr_component, b as subscribe, d as each, e as escape } from "../../../chunks/index3.js";
import { o as order } from "../../../chunks/stores2.js";
const Page = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  let $order, $$unsubscribe_order;
  $$unsubscribe_order = subscribe(order, (value) => $order = value);
  $$unsubscribe_order();
  return `<div><h3 class="${"text-center bg-primary text-white p-2"}">Order Summary</h3>
	<div class="${"p-3"}"><table class="${"table table-sm table-striped"}"><thead><tr><th>Quantity</th>
					<th>Product</th>
					<th class="${"text-end"}">Price</th>
					<th class="${"text-end"}">Subtotal</th></tr></thead>
			<tbody>${each($order.orderLines, (line) => {
    return `<tr><td>${escape(line.quantity)}</td>
						<td>${escape(line.product.name)}</td>
						<td class="${"text-end"}">$${escape(line.product.price.toFixed(2))}</td>
						<td class="${"text-end"}">$${escape(line.total.toFixed(2))}</td>
					</tr>`;
  })}</tbody>
			<tfoot><tr><th class="${"text-end"}" colspan="${"3"}">Total:</th>
					<th class="${"text-end"}"><span style="${"display: inline-block"}">$${escape($order.total.toFixed(2))}</span></th></tr></tfoot></table></div>
	<div class="${"text-center"}"><a href="${"/products"}" class="${"btn btn-secondary m-1"}">Back </a>
		<button class="${"btn btn-primary m-1"}">Submit Order</button></div></div>`;
});
export {
  Page as default
};
