import { c as create_ssr_component } from "../../chunks/index3.js";
const bootstrap_min = "";
const Layout = create_ssr_component(($$result, $$props, $$bindings, slots) => {
  return `${$$result.head += `<!-- HEAD_svelte-jjba08_START -->${$$result.title = `<title>Svelte app</title>`, ""}<!-- HEAD_svelte-jjba08_END -->`, ""}

${slots.default ? slots.default({}) : ``}`;
});
export {
  Layout as default
};
