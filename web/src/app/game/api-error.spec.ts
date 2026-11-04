import { HttpErrorResponse } from '@angular/common/http';
import { TimeoutError } from 'rxjs';

import { classifyError } from './api-error';

function httpError(status: number, error?: string): HttpErrorResponse {
  return new HttpErrorResponse({
    status,
    error: error ? { ok: false, error, message: '...' } : null,
  });
}

describe('classifyError', () => {
  it('sin respuesta útil (red caída, 5xx, timeout) es offline', () => {
    expect(classifyError(httpError(0))).toBe('offline');
    expect(classifyError(httpError(502))).toBe('offline');
    expect(classifyError(new TimeoutError())).toBe('offline');
  });

  it('traduce los errores del contrato', () => {
    expect(classifyError(httpError(400, 'illegal_move'))).toBe('illegal_move');
    expect(classifyError(httpError(409, 'game_over'))).toBe('conflict');
    expect(classifyError(httpError(409, 'not_your_turn'))).toBe('conflict');
    expect(classifyError(httpError(404, 'unknown_game'))).toBe('lost');
    expect(classifyError(httpError(400, 'validation'))).toBe('other');
  });

  it('un error que no viene de HTTP no se confunde con falta de conexión', () => {
    expect(classifyError(new Error('boom'))).toBe('other');
  });
});
