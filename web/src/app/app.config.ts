import { provideHttpClient } from '@angular/common/http';
import { ApplicationConfig, inject, provideBrowserGlobalErrorListeners } from '@angular/core';

import { FakeGameApiService, USING_FAKE_API } from './game/fake-game-api.service';
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
        inject(USING_FAKE_API) ? inject(FakeGameApiService) : inject(GameApiService),
    },
  ],
};
