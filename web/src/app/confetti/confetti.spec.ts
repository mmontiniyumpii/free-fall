import { TestBed } from '@angular/core/testing';

import { Confetti } from './confetti';

describe('Confetti', () => {
  function render(): HTMLElement[] {
    const fixture = TestBed.createComponent(Confetti);
    fixture.detectChanges();
    return Array.from((fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('.piece'));
  }

  it('reparte 90 piezas a lo ancho de toda la zona', () => {
    const left = render().map((piece) => Number.parseFloat(piece.style.left));

    expect(left.length).toBe(90);
    expect(Math.min(...left)).toBeLessThan(10);
    expect(Math.max(...left)).toBeGreaterThan(90);
  });

  it('es siempre igual: no usa aleatoriedad', () => {
    const styles = (pieces: HTMLElement[]) => pieces.map((piece) => piece.getAttribute('style'));

    expect(styles(render())).toEqual(styles(render()));
  });
});
