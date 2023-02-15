import { c as create_ssr_component, f as createEventDispatcher, e as escape, b as subscribe, d as each, h as add_attribute, v as validate_component } from "../../../chunks/index3.js";
import { s as selectedCategory, o as order, p as products } from "../../../chunks/stores2.js";
const ProductItem = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  createEventDispatcher();
  let { product } = $$props;
  if ($$props.product === void 0 && $$bindings.product && product !== void 0)
    $$bindings.product(product);
  return `<div class="${"card m-1 p-1 bg-light"}"><h4>${escape(product.name)}
		<span class="${"badge rounded-pill bg-primary float-end"}">$${escape(product.price.toFixed(2))}</span></h4>
	<div class="${"card-text bg-white p-1"}">${escape(product.description)}
		<button class="${"btn btn-success btn-sm float-end"}">Add To Cart
		</button>
		<select class="${"form-control-inline float-end m-1"}"><option value="${"1"}">1</option><option value="${"2"}">2</option><option value="${"3"}">3</option></select></div></div>`;
});
const CategoryList = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  let $selectedCategory, $$unsubscribe_selectedCategory;
  $$unsubscribe_selectedCategory = subscribe(selectedCategory, (value) => $selectedCategory = value);
  createEventDispatcher();
  let { categories = [] } = $$props;
  const getButtonClasses = (category) => {
    const btnClass = $selectedCategory === category ? "btn-primary" : "btn-secondary";
    return `btn ${btnClass}`;
  };
  if ($$props.categories === void 0 && $$bindings.categories && categories !== void 0)
    $$bindings.categories(categories);
  $$unsubscribe_selectedCategory();
  return `<div class="${"d-grid gap-2"}">${each(categories, (c) => {
    return `<button${add_attribute("class", getButtonClasses(c), 0)}>${escape(c)}
		</button>`;
  })}</div>`;
});
const Header = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  let count;
  let displayText1;
  let displayText2;
  let $order, $$unsubscribe_order;
  $$unsubscribe_order = subscribe(order, (value) => $order = value);
  count = $order.productCount || 0;
  displayText1 = count === 0 ? "(No Selection)" : `${count} product(s), `;
  displayText2 = count === 0 ? "" : `$${$order.total.toFixed(2)}`;
  $$unsubscribe_order();
  return `<div class="${"p-1 bg-secondary text-white text-end"}"><span style="${"display: inline-block"}">${escape(displayText1)}</span>
	<span style="${"display: inline-block"}">${escape(displayText2)}</span>
	<a href="${"/order"}" class="${"btn btn-sm btn-primary m-1"}">Submit Order </a></div>`;
});
const Page = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  let categories;
  let filteredProducts;
  let $$unsubscribe_order;
  let $selectedCategory, $$unsubscribe_selectedCategory;
  let $products, $$unsubscribe_products;
  $$unsubscribe_order = subscribe(order, (value) => value);
  $$unsubscribe_selectedCategory = subscribe(selectedCategory, (value) => $selectedCategory = value);
  $$unsubscribe_products = subscribe(products, (value) => $products = value);
  categories = ["All", ...new Set($products.map((p) => p.category))];
  filteredProducts = $products.filter((p) => $selectedCategory === "All" || $selectedCategory === p.category);
  $$unsubscribe_order();
  $$unsubscribe_selectedCategory();
  $$unsubscribe_products();
  return `<div>${validate_component(Header, "Header").$$render($$result, {}, {}, {})}
	<div class="${"container-fluid"}"><div class="${"row"}"><div class="${"col-3 p-2"}">${validate_component(CategoryList, "CategoryList").$$render($$result, { categories }, {}, {})}</div>
			<div class="${"col-9 p-2"}">${each(filteredProducts, (product) => {
    return `<div>${validate_component(ProductItem, "ProductItem").$$render($$result, { product }, {}, {})}
					</div>`;
  })}</div></div></div></div>`;
});
export {
  Page as default
};
