// Cast-free access to a DOM event's target. `event.target` is typed
// `EventTarget | null`, so reading `.value` / passing it to `Node.contains`
// normally needs an `as` cast — which erases the check and mishandles a null
// target. These narrow with `instanceof` instead, so the access is a real
// runtime check and null degrades safely (empty string / null node).

export const targetValue = (event: Event): string =>
    event.target instanceof HTMLInputElement
    || event.target instanceof HTMLSelectElement
        ? event.target.value
        : '';

export const targetNode = (event: Event): Node | null =>
    event.target instanceof Node ? event.target : null;
