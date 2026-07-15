// Pure edge-shadow math, extracted from the component so it is unit-testable
// without a real layout engine (jsdom reports zero sizes).
export type EdgeState = {
    left: boolean;
    right: boolean;
};

export const edgeState = (scrollLeft: number, scrollWidth: number, clientWidth: number): EdgeState => {
    const maxScroll = scrollWidth - clientWidth;

    if (maxScroll <= 0) {
        return { left: false, right: false };
    }

    // 1px tolerance: browsers report fractional scroll positions near the edges.
    return {
        left: scrollLeft > 1,
        right: scrollLeft < maxScroll - 1,
    };
};
