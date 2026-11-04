import { provideHttpClient } from '@angular/common/http';
import {
  ApplicationConfig,
  DOCUMENT,
  inject,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';

import { FakeGameApiService, fakeRequested } from './game/fake-game-api.service';
import { GameApi } from './game/game-api';
import { GameApiService } from './game/game-api.service';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideHttpClient(),
    GameApiService,
    FakeGameApiService,
    {
      provide: GameApi,
      useFactory: () =>
        fakeRequested(inject(DOCUMENT).location.search)
          ? inject(FakeGameApiService)
          : inject(GameApiService),
    },
  ],
};
