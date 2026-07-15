import { describe, expect, it } from 'vitest';
import { edgeState } from '@/app-ui/ScrollShadow/edgeState';

// The pure math behind the scroll fade hints — extracted precisely because
// jsdom has no layout engine to drive the component's measurements.
describe('edgeState', () => {
    it('shows no shadows when the content fits', () => {
        expect(edgeState(0, 800, 800)).toEqual({ left: false, right: false });
        expect(edgeState(0, 500, 800)).toEqual({ left: false, right: false });
    });

    it('shows only the right shadow at the left edge', () => {
        expect(edgeState(0, 1200, 800)).toEqual({ left: false, right: true });
    });

    it('shows both shadows mid-scroll', () => {
        expect(edgeState(200, 1200, 800)).toEqual({ left: true, right: true });
    });

    it('shows only the left shadow at the right edge', () => {
        expect(edgeState(400, 1200, 800)).toEqual({ left: true, right: false });
        expect(edgeState(399.5, 1200, 800)).toEqual({ left: true, right: false });
    });

    it('tolerates fractional scroll positions near the edges', () => {
        expect(edgeState(0.5, 1200, 800)).toEqual({ left: false, right: true });
    });
});
