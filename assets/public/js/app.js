/*
 * Ferret
 * Copyright (c) 2016 Yieldbot, Inc.
 * For the full copyright and license information, please view the LICENSE.txt file.
 */

/* jslint browser: true */
/* global document: false, $: false, Rx: false */
'use strict';

// Create the module
var app = function app() {

  if(typeof $ != "function" || typeof Rx != "object") {
    throw new Error("missing or invalid libraries (jQuery, Rx)");
  }

  // Init vars
  var serverUrl = location.protocol + '//' + location.hostname + ':' + location.port;

  // Get dom elements
  var $inputElm           = $('#searchInput'),
      $resultsElm         = $('#searchResults'),
      $logoMain           = $("#logoMain"),
      $logoNavbarHolder   = $('#logoNavbarHolder'),
      $searchMain         = $("#searchMain"),
      $searchNavbarHolder = $("#searchNavbarHolder");


  // init initializes the app
  function init() {

    // Create an observable from the keyup event
    var observable = Rx.Observable
      .fromEvent($inputElm, 'keyup')
      .filter(function(e) { return (e.keyCode == 13); })
      .map(function(e) { return e.target.value; })
      .filter(function(text) { return text.length > 2; })
      .distinctUntilChanged()
      .throttle(1000);

    // Iterate search providers
    ['answerhub', 'trello', 'slack', 'github'].forEach(function(provider) {
      observable.flatMapLatest(function(keyword) {
        beforeSearch();
        return search(provider, keyword);
      })
      .subscribe(
        function(data) {
          afterSearch(null, {provider: provider, data: data});
        },
        function(err) {
          afterSearch(err, {provider: provider});
        }
      );
    });

    $inputElm.focus();
  }

  // search makes a search by the given provider and keyword
  function search(provider, keyword) {
    serverUrl = (location.protocol == 'file:') ? 'http://localhost:3030' : serverUrl; // for debug

    return $.ajax({
      url:      serverUrl+'/search',
      dataType: 'jsonp',
      method:   'GET',
      data: {
        provider: (''+provider),
        keyword:  (''+keyword),
        timeout:  '5000ms'
      }
    }).promise();
  }

  // beforeSearch prepares UI before search event
  function beforeSearch() {
    $logoMain.detach().appendTo($logoNavbarHolder).addClass('logo-navbar');
    $searchMain.detach().appendTo($searchNavbarHolder).addClass('input-group-search-navbar');
    $resultsElm.empty();
  }

  // afterSearch prepares UI after search event
  function afterSearch(err, result) {
    var provider = (result && result.provider || 'unknown');

    if(err) {
      var errMsg = 'unknown error';
      if(typeof err == 'object') {
        errMsg = err.statusText || errMsg;
        if(typeof err.responseJSON == 'object') {
          errMsg = err.responseJSON.message || err.responseJSON.error || errMsg;
        }
      }
      $resultsElm.append($('<h3>').text(provider));
      $resultsElm.append($('<div class="alert alert-danger" role="alert">').text('search failed due to ' + errMsg));
      return;
    }

    if(result && typeof result == 'object' && result.data instanceof Array) {
      $resultsElm.append($('<h3>').text(provider));
      $resultsElm.append($.map(result.data, function (v) {
        return $('<li>').html('<a href="'+v.Link+'" target="_blank">'+v.Description+'</a>');
      }));
      $resultsElm.append($('<hr>'));
    }

    return;
  }

  // Return
  return {
    init: init
  };
};

$(document).ready(function() {
  app().init();
});