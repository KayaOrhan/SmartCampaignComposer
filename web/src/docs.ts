import SwaggerUI from 'swagger-ui-dist/swagger-ui-bundle.js'
import 'swagger-ui-dist/swagger-ui.css'

SwaggerUI({
  url: '/api/openapi.json',
  dom_id: '#swagger-ui',
  validatorUrl: null,
  // Read-only exploration; campaign writes require the application's human review.
  supportedSubmitMethods: ['get'],
  deepLinking: true,
})
