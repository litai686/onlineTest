import { store } from "./store.js";
import { router } from "./router.js";

store.load();
window.addEventListener("hashchange", router);
window.addEventListener("unhandledrejection", (event) => {
  console.error(event.reason);
});
router();
