import { env } from "./config/env.js";
import { app } from "./app.js";

const port = env.PORT;

app.listen(port, () => {
  console.log(`Edge Evaluator is running on port http://localhost:${port}`);
});