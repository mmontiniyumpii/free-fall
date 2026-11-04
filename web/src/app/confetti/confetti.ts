import { ChangeDetectionStrategy, Component } from '@angular/core';

interface Piece {
  leftPct: number;
  sway: number;
  spin: number;
  delayMs: number;
  durationMs: number;
  color: string;
  shape: 'strip' | 'square' | 'wide';
}

const PIECES = 90;
const COLORS = [
  'var(--color-accent)',
  'var(--color-confetti-light)',
  'var(--color-confetti-warm)',
  'var(--color-bot)',
];
const SHAPES: Piece['shape'][] = ['strip', 'square', 'wide'];
/** Razón áurea: reparte las piezas sin aleatoriedad, así el confeti es siempre igual. */
const GOLDEN = 0.618034;

/** Fracción "pseudoaleatoria" estable para la pieza i (0 … 1). */
function spread(i: number, salt: number): number {
  return (i * GOLDEN * salt) % 1;
}

/**
 * Lluvia de confeti al ganar, solo con CSS. Cubre el contenedor que la aloje
 * (la zona del tablero) y no bloquea clics.
 */
@Component({
  selector: 'app-confetti',
  template: `
    @for (piece of pieces; track $index) {
      <span
        [class]="'piece piece--' + piece.shape"
        [style.left.%]="piece.leftPct"
        [style.--sway]="piece.sway + 'px'"
        [style.--spin]="piece.spin + 'deg'"
        [style.--delay]="piece.delayMs + 'ms'"
        [style.--duration]="piece.durationMs + 'ms'"
        [style.background]="piece.color"
      ></span>
    }
  `,
  styleUrl: './confetti.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { 'aria-hidden': 'true' },
})
export class Confetti {
  protected readonly pieces: Piece[] = Array.from({ length: PIECES }, (_, i) => ({
    leftPct: Math.round(spread(i, 1) * 1000) / 10,
    sway: Math.round((spread(i, 3) * 2 - 1) * 70),
    spin: Math.round((spread(i, 5) * 2 - 1) * 900),
    delayMs: Math.round(spread(i, 7) * 900),
    durationMs: 2200 + Math.round(spread(i, 11) * 1300),
    color: COLORS[i % COLORS.length],
    shape: SHAPES[i % SHAPES.length],
  }));
}
