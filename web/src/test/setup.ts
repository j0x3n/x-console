import { configure } from "@testing-library/react";

// Lazy page imports and jsdom rendering take longer on busy CI runners.
configure({ asyncUtilTimeout: 15_000 });
