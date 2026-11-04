import { DOCUMENT, InjectionToken, inject } from '@angular/core';

/**
 * El dispositivo no tiene hover (teléfono, tablet): el tablero usa toque en dos
 * pasos. Depende de la capacidad, no del ancho: una ventana angosta en la
 * compu sigue soltando con un clic.
 */
export const TOUCH_INPUT = new InjectionToken<boolean>('TOUCH_INPUT', {
  factory: () => inject(DOCUMENT).defaultView?.matchMedia?.('(hover: none)').matches ?? false,
});
